package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStickers(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, _ := s.member(t, "ann")
	bob, _ := s.member(t, "bob")
	anonymous := context.Background()

	// The site has no packs of its own: Bob sells a free one and one for a vote.
	if catalog, err := s.client.GetStickerCatalog(anonymous, &userv1.GetStickerCatalogRequest{}); err != nil || len(catalog.GetPacks()) != 0 {
		t.Fatalf("GetStickerCatalog = %v, %v", catalog, err)
	}
	for _, price := range []int64{0, 1} {
		if _, err := s.client.CreateStickerPack(bob, &userv1.CreateStickerPackRequest{Name: "Котики", Price: price, Published: true, PhotoIds: []string{uuid.NewString()}}); err != nil {
			t.Fatalf("CreateStickerPack: %v", err)
		}
	}

	// The catalogue is public.
	catalog, err := s.client.GetStickerCatalog(anonymous, &userv1.GetStickerCatalogRequest{})
	if err != nil || len(catalog.GetPacks()) != 2 || len(catalog.GetPacks()[0].GetStickers()) == 0 || catalog.GetPacks()[0].GetAdded() {
		t.Fatalf("GetStickerCatalog = %v, %v", catalog, err)
	}
	free, dear := catalog.GetPacks()[0].GetId(), catalog.GetPacks()[1].GetId()
	if catalog.GetPacks()[0].GetPrice() != 0 {
		free, dear = dear, free
	}
	first := func(packs []*userv1.StickerPack) *userv1.StickerPack {
		for _, p := range packs {
			if p.GetId() == free {
				return p
			}
		}

		return nil
	}

	if _, err = s.client.AddStickerPack(anonymous, &userv1.AddStickerPackRequest{PackId: free}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("AddStickerPack without a token: %v", err)
	}
	for _, pack := range []string{"dogs", "smiles"} {
		if _, err = s.client.AddStickerPack(ann, &userv1.AddStickerPackRequest{PackId: pack}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("AddStickerPack of an unknown pack %s: %v", pack, err)
		}
	}
	if _, err = s.client.AddStickerPack(ann, &userv1.AddStickerPackRequest{PackId: free}); err != nil {
		t.Fatalf("AddStickerPack: %v", err)
	}

	mine, err := s.client.GetStickerCatalog(ann, &userv1.GetStickerCatalogRequest{})
	if err != nil || !first(mine.GetPacks()).GetAdded() || !first(mine.GetPacks()).GetBought() {
		t.Fatalf("GetStickerCatalog of ann = %v, %v", mine, err)
	}

	if _, err = s.client.AddStickerPack(ann, &userv1.AddStickerPackRequest{PackId: dear}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("AddStickerPack without votes: %v", err)
	}

	if _, err = s.client.RemoveStickerPack(ann, &userv1.RemoveStickerPackRequest{PackId: free}); err != nil {
		t.Fatalf("RemoveStickerPack: %v", err)
	}
	if mine, err = s.client.GetStickerCatalog(ann, &userv1.GetStickerCatalogRequest{}); err != nil || first(mine.GetPacks()).GetAdded() {
		t.Fatalf("a pack put away = %v, %v", mine, err)
	}
}

func TestStickerStudio(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, _ := s.member(t, "ann")
	bob, _ := s.member(t, "bob")
	photo := uuid.NewString()

	if _, err := s.client.CreateStickerPack(ann, &userv1.CreateStickerPackRequest{Name: " ", PhotoIds: []string{photo}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateStickerPack without a name: %v", err)
	}
	if _, err := s.client.CreateStickerPack(ann, &userv1.CreateStickerPackRequest{Name: "Котики", PhotoIds: []string{"nope"}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateStickerPack of a bad photo: %v", err)
	}
	if _, err := s.client.CreateStickerPack(ann, &userv1.CreateStickerPackRequest{Name: "Котики", Price: -1, PhotoIds: []string{photo}}); status.Code(err) != codes.InvalidArgument ||
		status.Convert(err).Message() != "invalid sticker pack price" {
		t.Fatalf("CreateStickerPack with a negative price: %v", err)
	}

	created, err := s.client.CreateStickerPack(ann, &userv1.CreateStickerPackRequest{Name: "Котики", Price: 1, Published: true, PhotoIds: []string{photo}})
	if err != nil || created.GetPack().GetName() != "Котики" || !created.GetPack().GetAdded() || created.GetPack().GetAuthorId() == "" {
		t.Fatalf("CreateStickerPack = %v, %v", created, err)
	}
	pack := created.GetPack()

	catalog, err := s.client.GetStickerCatalog(context.Background(), &userv1.GetStickerCatalogRequest{})
	if err != nil || catalog.GetPacks()[len(catalog.GetPacks())-1].GetId() != pack.GetId() {
		t.Fatalf("the shop = %v, %v", catalog, err)
	}

	authors, err := s.client.GetStickerAuthors(context.Background(), &userv1.GetStickerAuthorsRequest{Stickers: pack.GetStickers()})
	if err != nil || authors.GetAuthors()[pack.GetStickers()[0]] != pack.GetAuthorId() {
		t.Fatalf("GetStickerAuthors = %v, %v", authors, err)
	}

	if _, err = s.client.EditStickerPack(bob, &userv1.EditStickerPackRequest{PackId: pack.GetId(), Name: "Мои"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("EditStickerPack by another: %v", err)
	}
	if _, err = s.client.EditStickerPack(ann, &userv1.EditStickerPackRequest{PackId: "smiles", Name: "Мои"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("EditStickerPack of the catalogue: %v", err)
	}

	added := uuid.NewString()
	edited, err := s.client.EditStickerPack(ann, &userv1.EditStickerPackRequest{
		PackId: pack.GetId(), Name: "Кошки", Price: 1, AddPhotoIds: []string{added}, RemovePhotoIds: []string{photo},
	})
	if err != nil || edited.GetPack().GetName() != "Кошки" || len(edited.GetRemovedPhotoIds()) != 1 || edited.GetRemovedPhotoIds()[0] != photo {
		t.Fatalf("EditStickerPack = %v, %v", edited, err)
	}

	mine, err := s.client.ListMyStickerPacks(ann, &userv1.ListMyStickerPacksRequest{})
	if err != nil || len(mine.GetPacks()) != 1 || mine.GetPacks()[0].GetHolders() != 0 {
		t.Fatalf("ListMyStickerPacks = %v, %v", mine, err)
	}

	deleted, err := s.client.DeleteStickerPack(ann, &userv1.DeleteStickerPackRequest{PackId: pack.GetId()})
	if err != nil || len(deleted.GetRemovedPhotoIds()) != 1 || deleted.GetRemovedPhotoIds()[0] != added {
		t.Fatalf("DeleteStickerPack = %v, %v", deleted, err)
	}
	if _, err = s.client.DeleteStickerPack(ann, &userv1.DeleteStickerPackRequest{PackId: pack.GetId()}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("DeleteStickerPack of none: %v", err)
	}
}
