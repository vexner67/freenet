package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStickers(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, _ := s.member(t, "ann")
	anonymous := context.Background()

	// The catalogue is public.
	catalog, err := s.client.GetStickerCatalog(anonymous, &userv1.GetStickerCatalogRequest{})
	if err != nil || len(catalog.GetPacks()) == 0 || len(catalog.GetPacks()[0].GetStickers()) == 0 || catalog.GetPacks()[0].GetAdded() {
		t.Fatalf("GetStickerCatalog = %v, %v", catalog, err)
	}
	free := catalog.GetPacks()[0].GetId()

	if _, err = s.client.AddStickerPack(anonymous, &userv1.AddStickerPackRequest{PackId: free}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("AddStickerPack without a token: %v", err)
	}
	if _, err = s.client.AddStickerPack(ann, &userv1.AddStickerPackRequest{PackId: "dogs"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("AddStickerPack of an unknown pack: %v", err)
	}
	if _, err = s.client.AddStickerPack(ann, &userv1.AddStickerPackRequest{PackId: free}); err != nil {
		t.Fatalf("AddStickerPack: %v", err)
	}

	mine, err := s.client.GetStickerCatalog(ann, &userv1.GetStickerCatalogRequest{})
	if err != nil || !mine.GetPacks()[0].GetAdded() || !mine.GetPacks()[0].GetBought() {
		t.Fatalf("GetStickerCatalog of ann = %v, %v", mine, err)
	}

	for _, p := range mine.GetPacks() {
		if p.GetPrice() == 0 {
			continue
		}
		if _, err = s.client.AddStickerPack(ann, &userv1.AddStickerPackRequest{PackId: p.GetId()}); status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("AddStickerPack without votes: %v", err)
		}
		break
	}

	if _, err = s.client.RemoveStickerPack(ann, &userv1.RemoveStickerPackRequest{PackId: free}); err != nil {
		t.Fatalf("RemoveStickerPack: %v", err)
	}
	if mine, err = s.client.GetStickerCatalog(ann, &userv1.GetStickerCatalogRequest{}); err != nil || mine.GetPacks()[0].GetAdded() {
		t.Fatalf("a pack put away = %v, %v", mine, err)
	}
}
