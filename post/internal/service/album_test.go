package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

func newAlbumService(t *testing.T, moderators fakeModerators) (*service.AlbumService, *fakePhotos) {
	t.Helper()

	albums := inmem.NewAlbumRepository()
	photos := &fakePhotos{}

	return service.NewAlbumService(service.AlbumDeps{
		Albums: albums, Photos: albums.Photos(), Tx: fakeTx{}, Media: photos, Moderators: moderators,
	}), photos
}

// newPhotos is n photos to add, IDs growing as uploads do.
func newPhotos(n int) []service.NewAlbumPhoto {
	photos := make([]service.NewAlbumPhoto, n)
	for i := range photos {
		photos[i] = service.NewAlbumPhoto{ID: uuid.Must(uuid.NewV7())}
	}

	return photos
}

func TestAlbumService_Create(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, ann := uuid.New(), uuid.New(), uuid.New()
	svc, _ := newAlbumService(t, fakeModerators{{club, admin}: true})

	own, err := svc.Create(ctx, service.CreateAlbumInput{OwnerID: ann, RequesterID: ann, Name: "Лето", Description: "На даче"})
	if err != nil || own.OwnerID() != ann || own.Description() != "На даче" {
		t.Fatalf("Create = %+v, %v", own, err)
	}

	clubs, err := svc.Create(ctx, service.CreateAlbumInput{OwnerID: club, RequesterID: admin, Name: "Кошки"})
	if err != nil || clubs.OwnerID() != club {
		t.Fatalf("an admin creates for the club: %+v, %v", clubs, err)
	}

	for _, owner := range []uuid.UUID{club, admin} {
		if _, err = svc.Create(ctx, service.CreateAlbumInput{OwnerID: owner, RequesterID: ann, Name: "x"}); !errors.Is(err, post.ErrCannotChangeAlbum) {
			t.Fatalf("ann creates an album of %v: %v", owner, err)
		}
	}
}

func TestAlbumService_ListAndPhotos(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann := uuid.New()
	svc, photos := newAlbumService(t, fakeModerators{})

	summer, _ := svc.Create(ctx, service.CreateAlbumInput{OwnerID: ann, RequesterID: ann, Name: "Лето"})
	autumn, _ := svc.Create(ctx, service.CreateAlbumInput{OwnerID: ann, RequesterID: ann, Name: "Осень"})
	empty, _ := svc.Create(ctx, service.CreateAlbumInput{OwnerID: ann, RequesterID: ann, Name: "Пусто"})

	added := newPhotos(3)
	added[1].Description = "Море"
	got, err := svc.AddPhotos(ctx, ann, summer.ID(), added)
	if err != nil || len(got) != 3 || got[1].Description() != "Море" || got[0].AuthorID() != ann {
		t.Fatalf("AddPhotos = %+v, %v", got, err)
	}
	if want := []photoCall{{ann, []uuid.UUID{added[0].ID, added[1].ID, added[2].ID}}}; !reflect.DeepEqual(photos.published, want) || photos.outsideTx != 0 {
		t.Fatalf("published %+v (%d outside the transaction), want %+v", photos.published, photos.outsideTx, want)
	}
	if _, err = svc.AddPhotos(ctx, ann, autumn.ID(), newPhotos(1)); err != nil {
		t.Fatal(err)
	}

	// Newest first, two a page, each with its first photo as the cover.
	page, err := svc.List(ctx, ann, uuid.Nil, 2)
	if err != nil || page.Total != 3 || len(page.Albums) != 2 || page.Albums[0].ID() != empty.ID() || page.NextCursor != autumn.ID() {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	if page.Covers[empty.ID()] != nil || page.Covers[autumn.ID()] == nil {
		t.Fatalf("covers = %+v", page.Covers)
	}
	page, err = svc.List(ctx, ann, page.NextCursor, 2)
	if err != nil || len(page.Albums) != 1 || page.Albums[0].Photos() != 3 || page.Covers[summer.ID()].ID() != added[0].ID || page.NextCursor != uuid.Nil {
		t.Fatalf("second page = %+v, %v", page, err)
	}

	// Oldest first.
	shots, err := svc.Photos(ctx, summer.ID(), uuid.Nil, 2)
	if err != nil || shots.Total != 3 || len(shots.Photos) != 2 || shots.Photos[0].ID() != added[0].ID || shots.NextCursor != added[1].ID {
		t.Fatalf("photos = %+v, %v", shots, err)
	}
	shots, _ = svc.Photos(ctx, summer.ID(), shots.NextCursor, 2)
	if len(shots.Photos) != 1 || shots.Photos[0].ID() != added[2].ID || shots.NextCursor != uuid.Nil {
		t.Fatalf("photos after = %+v", shots)
	}

	p, a, err := svc.Photo(ctx, added[1].ID)
	if err != nil || p.Description() != "Море" || a.ID() != summer.ID() {
		t.Fatalf("Photo = %+v, %+v, %v", p, a, err)
	}

	if _, err = svc.Photos(ctx, uuid.New(), uuid.Nil, 2); !errors.Is(err, post.ErrAlbumNotFound) {
		t.Fatalf("photos of no album: %v", err)
	}
	if _, _, err = svc.Photo(ctx, uuid.New()); !errors.Is(err, post.ErrAlbumPhotoNotFound) {
		t.Fatalf("no photo: %v", err)
	}
}

func TestAlbumService_AddPhotos(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, ann := uuid.New(), uuid.New(), uuid.New()
	svc, photos := newAlbumService(t, fakeModerators{{club, admin}: true})
	own, _ := svc.Create(ctx, service.CreateAlbumInput{OwnerID: ann, RequesterID: ann, Name: "Лето"})
	clubs, _ := svc.Create(ctx, service.CreateAlbumInput{OwnerID: club, RequesterID: admin, Name: "Кошки"})

	twice := newPhotos(1)
	for _, tt := range []struct {
		name      string
		requester uuid.UUID
		album     uuid.UUID
		photos    []service.NewAlbumPhoto
		err       error
	}{
		{"none", ann, own.ID(), nil, post.ErrNoAlbumPhotos},
		{"more than 10", ann, own.ID(), newPhotos(11), service.ErrTooManyAlbumPhotos},
		{"the same twice", ann, own.ID(), append(twice, twice...), post.ErrDuplicatePhoto},
		{"a stranger", admin, own.ID(), newPhotos(1), post.ErrCannotChangeAlbum},
		{"a member of the club", ann, clubs.ID(), newPhotos(1), post.ErrCannotChangeAlbum},
		{"no album", ann, uuid.New(), newPhotos(1), post.ErrAlbumNotFound},
		{"10", ann, own.ID(), newPhotos(10), nil},
		{"an admin of the club", admin, clubs.ID(), newPhotos(1), nil},
	} {
		if _, err := svc.AddPhotos(ctx, tt.requester, tt.album, tt.photos); !errors.Is(err, tt.err) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.err)
		}
	}

	// A photo is in one album at most.
	again := newPhotos(1)
	if _, err := svc.AddPhotos(ctx, ann, own.ID(), again); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddPhotos(ctx, ann, own.ID(), again); !errors.Is(err, post.ErrDuplicatePhoto) {
		t.Fatalf("added again: %v", err)
	}

	// A refused media call adds nothing.
	photos.err = errors.New("media is down")
	if _, err := svc.AddPhotos(ctx, ann, own.ID(), newPhotos(1)); err == nil {
		t.Fatal("added without media")
	}
}

func TestAlbumService_ChangeAndDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, bob, ann := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	svc, photos := newAlbumService(t, fakeModerators{{club, admin}: true, {club, bob}: true})
	own, _ := svc.Create(ctx, service.CreateAlbumInput{OwnerID: ann, RequesterID: ann, Name: "Лето"})

	renamed, err := svc.Change(ctx, ann, own.ID(), "Осень", "Листья")
	if err != nil || renamed.Name() != "Осень" {
		t.Fatalf("Change = %+v, %v", renamed, err)
	}
	if got, _, _ := svc.Get(ctx, own.ID()); got.Name() != "Осень" || got.Description() != "Листья" {
		t.Fatalf("Get after Change = %+v", got)
	}
	if _, err = svc.Change(ctx, bob, own.ID(), "x", ""); !errors.Is(err, post.ErrCannotChangeAlbum) {
		t.Fatalf("a stranger renames: %v", err)
	}
	if err = svc.Delete(ctx, bob, own.ID()); !errors.Is(err, post.ErrCannotChangeAlbum) {
		t.Fatalf("a stranger deletes: %v", err)
	}

	// An album of the club with photos of two admins, eleven of one: media
	// takes ten a call, each under who uploaded it.
	clubs, _ := svc.Create(ctx, service.CreateAlbumInput{OwnerID: club, RequesterID: admin, Name: "Кошки"})
	admins, bobs := newPhotos(10), newPhotos(1)
	if _, err = svc.AddPhotos(ctx, admin, clubs.ID(), admins); err != nil {
		t.Fatal(err)
	}
	more := newPhotos(1)
	if _, err = svc.AddPhotos(ctx, admin, clubs.ID(), more); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.AddPhotos(ctx, bob, clubs.ID(), bobs); err != nil {
		t.Fatal(err)
	}

	if err = svc.Delete(ctx, bob, clubs.ID()); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.Get(ctx, clubs.ID()); !errors.Is(err, post.ErrAlbumNotFound) {
		t.Fatalf("deleted album: %v", err)
	}

	byOwner := make(map[uuid.UUID]int)
	for _, c := range photos.deleted {
		if len(c.photos) > service.MaxPhotosAtOnce {
			t.Fatalf("%d photos a call", len(c.photos))
		}
		byOwner[c.owner] += len(c.photos)
	}
	if byOwner[admin] != 11 || byOwner[bob] != 1 || len(photos.deleted) != 3 || photos.outsideTx != 0 {
		t.Fatalf("deleted %+v", photos.deleted)
	}
}

func TestAlbumService_DescribeAndDeletePhoto(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, ann := uuid.New(), uuid.New(), uuid.New()
	svc, photos := newAlbumService(t, fakeModerators{{club, admin}: true, {club, ann}: true})
	clubs, _ := svc.Create(ctx, service.CreateAlbumInput{OwnerID: club, RequesterID: admin, Name: "Кошки"})
	anns := newPhotos(2)
	if _, err := svc.AddPhotos(ctx, ann, clubs.ID(), anns); err != nil {
		t.Fatal(err)
	}

	// Only who uploaded it describes it, even to the admins.
	described, err := svc.Describe(ctx, ann, anns[0].ID, "Барсик")
	if err != nil || described.Description() != "Барсик" {
		t.Fatalf("Describe = %+v, %v", described, err)
	}
	if _, err = svc.Describe(ctx, admin, anns[0].ID, "x"); !errors.Is(err, post.ErrCannotDescribePhoto) {
		t.Fatalf("an admin describes: %v", err)
	}
	if p, _, _ := svc.Photo(ctx, anns[0].ID); p.Description() != "Барсик" {
		t.Fatalf("Photo after Describe = %+v", p)
	}

	// Who uploaded it deletes it; so does whoever may change the album.
	if err = svc.DeletePhoto(ctx, uuid.New(), anns[0].ID); !errors.Is(err, post.ErrCannotDeleteAlbumPhoto) {
		t.Fatalf("a stranger deletes: %v", err)
	}
	if err = svc.DeletePhoto(ctx, ann, anns[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = svc.DeletePhoto(ctx, admin, anns[1].ID); err != nil {
		t.Fatal(err)
	}
	if err = svc.DeletePhoto(ctx, admin, anns[1].ID); !errors.Is(err, post.ErrAlbumPhotoNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}

	if want := []photoCall{{ann, []uuid.UUID{anns[0].ID}}, {ann, []uuid.UUID{anns[1].ID}}}; !reflect.DeepEqual(photos.deleted, want) {
		t.Fatalf("deleted %+v, want %+v", photos.deleted, want)
	}
	if got, _, _ := svc.Get(ctx, clubs.ID()); got.Photos() != 0 {
		t.Fatalf("album = %+v; uncounted", got)
	}
}
