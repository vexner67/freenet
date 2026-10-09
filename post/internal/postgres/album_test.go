package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func newAlbumPhotos(t *testing.T, a *post.Album, author uuid.UUID, n int, now time.Time) []*post.AlbumPhoto {
	t.Helper()

	photos := make([]*post.AlbumPhoto, n)
	for i := range photos {
		p, err := post.NewAlbumPhoto(a, uuid.Must(uuid.NewV7()), author, "", now)
		if err != nil {
			t.Fatal(err)
		}
		photos[i] = p
	}

	return photos
}

func TestAlbumRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	albums := postgres.NewAlbumRepository(newPool(t))
	photos := albums.Photos()
	ann := uuid.New()
	start := time.Now().Add(-time.Hour)

	summer, err := post.NewAlbum(ann, "Лето", "На даче", start)
	if err != nil {
		t.Fatal(err)
	}
	autumn, _ := post.NewAlbum(ann, "Осень", "", start)
	for _, a := range []*post.Album{summer, autumn} {
		if err = albums.Save(ctx, a); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	if got, err := albums.Get(ctx, summer.ID()); err != nil || got.Stored() != summer.Stored() {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, summer.Stored())
	}

	// Photos count in their album and date it.
	added := newAlbumPhotos(t, summer, ann, 3, start)
	described, _ := added[1].Described(ann, "Море")
	added[1] = described
	later := start.Add(time.Minute)
	if err = photos.Add(ctx, summer.ID(), added, later); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got, _ := albums.Get(ctx, summer.ID()); got.Photos() != 3 || !got.UpdatedAt().Equal(later.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("after Add = %+v", got.Stored())
	}
	if got, err := photos.Get(ctx, added[1].ID()); err != nil || got.Description() != "Море" || got.AuthorID() != ann || got.AlbumID() != summer.ID() {
		t.Fatalf("Get photo = %+v, %v", got, err)
	}

	// A photo is in one album at most; nothing of a failed Add is kept.
	again := append(newAlbumPhotos(t, autumn, ann, 1, start), added[0])
	if err = photos.Add(ctx, autumn.ID(), again, later); !errors.Is(err, post.ErrDuplicatePhoto) {
		t.Fatalf("Add again: %v", err)
	}
	if got, _ := albums.Get(ctx, autumn.ID()); got.Photos() != 0 {
		t.Fatalf("after a failed Add = %+v", got.Stored())
	}
	if err = photos.Add(ctx, uuid.New(), newAlbumPhotos(t, autumn, ann, 1, start), later); !errors.Is(err, post.ErrAlbumNotFound) {
		t.Fatalf("Add to no album: %v", err)
	}

	list, err := albums.List(ctx, ann, uuid.Nil, 1)
	if err != nil || len(list) != 1 || list[0].ID() != autumn.ID() {
		t.Fatalf("List = %v, %v; newest first", list, err)
	}
	list, _ = albums.List(ctx, ann, autumn.ID(), 5)
	if len(list) != 1 || list[0].ID() != summer.ID() {
		t.Fatalf("List after = %v", list)
	}
	if n, err := albums.Count(ctx, ann); err != nil || n != 2 {
		t.Fatalf("Count = %d, %v", n, err)
	}

	page, err := photos.List(ctx, summer.ID(), added[0].ID(), 5)
	if err != nil || len(page) != 2 || page[0].ID() != added[1].ID() {
		t.Fatalf("photos after the first = %v, %v", page, err)
	}
	covers, err := photos.Covers(ctx, []uuid.UUID{summer.ID(), autumn.ID()})
	if err != nil || len(covers) != 1 || covers[0].ID() != added[0].ID() {
		t.Fatalf("Covers = %v, %v; the first photo, none for the empty album", covers, err)
	}

	renamed := summer.Renamed("Лето 2026", "", later.Add(time.Minute))
	if err = albums.Update(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if got, _ := albums.Get(ctx, summer.ID()); got.Name() != "Лето 2026" || got.Description() != "" || got.Photos() != 3 {
		t.Fatalf("after Update = %+v", got.Stored())
	}

	redescribed, _ := added[0].Described(ann, "Закат")
	if err = photos.Describe(ctx, redescribed); err != nil {
		t.Fatal(err)
	}
	if got, _ := photos.Get(ctx, added[0].ID()); got.Description() != "Закат" {
		t.Fatalf("after Describe = %+v", got)
	}

	if err = photos.Delete(ctx, added[2].ID(), later); err != nil {
		t.Fatal(err)
	}
	if err = photos.Delete(ctx, added[2].ID(), later); !errors.Is(err, post.ErrAlbumPhotoNotFound) {
		t.Fatalf("Delete twice: %v", err)
	}
	if got, _ := albums.Get(ctx, summer.ID()); got.Photos() != 2 {
		t.Fatalf("after Delete = %+v", got.Stored())
	}

	// An album takes its photos.
	gone, err := albums.Delete(ctx, summer.ID())
	if err != nil || len(gone) != 2 {
		t.Fatalf("Delete = %v, %v", gone, err)
	}
	if _, err = photos.Get(ctx, added[0].ID()); !errors.Is(err, post.ErrAlbumPhotoNotFound) {
		t.Fatalf("a photo of a deleted album: %v", err)
	}
	if _, err = albums.Delete(ctx, summer.ID()); !errors.Is(err, post.ErrAlbumNotFound) {
		t.Fatalf("Delete twice: %v", err)
	}
}

func TestAlbumRepository_DeleteByUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	albums := postgres.NewAlbumRepository(pool)
	tx := pgpool.NewTxManager(pool)
	club, ann, bob := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()

	anns, _ := post.NewAlbum(ann, "Анна", "", now)
	clubs, _ := post.NewAlbum(club, "Кошки", "", now)
	for _, a := range []*post.Album{anns, clubs} {
		if err := albums.Save(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := albums.Photos().Add(ctx, anns.ID(), newAlbumPhotos(t, anns, ann, 2, now), now); err != nil {
		t.Fatal(err)
	}
	bobs := newAlbumPhotos(t, clubs, bob, 1, now)
	if err := albums.Photos().Add(ctx, clubs.ID(), append(newAlbumPhotos(t, clubs, ann, 2, now), bobs...), now); err != nil {
		t.Fatal(err)
	}

	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		return albums.DeleteByUser(ctx, ann)
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = albums.Get(ctx, anns.ID()); !errors.Is(err, post.ErrAlbumNotFound) {
		t.Fatalf("ann's album: %v", err)
	}
	got, _ := albums.Get(ctx, clubs.ID())
	left, _ := albums.Photos().List(ctx, clubs.ID(), uuid.Nil, 10)
	if got.Photos() != 1 || len(left) != 1 || left[0].ID() != bobs[0].ID() {
		t.Fatalf("the club's album = %+v, %v; bob's photo only", got.Stored(), left)
	}
}

func TestAlbumRepository_AvatarAlbum(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	albums := postgres.NewAlbumRepository(newPool(t))
	ann := uuid.New()

	if _, err := albums.AvatarAlbum(ctx, ann); !errors.Is(err, post.ErrAlbumNotFound) {
		t.Fatalf("none yet: %v", err)
	}

	a, err := post.NewAvatarAlbum(ann, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = albums.Save(ctx, a); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := albums.AvatarAlbum(ctx, ann)
	if err != nil || got.ID() != a.ID() || got.Kind() != post.AlbumAvatars || got.Name() != a.Name() {
		t.Fatalf("AvatarAlbum = %+v, %v", got, err)
	}
	if got, err = albums.Get(ctx, a.ID()); err != nil || got.Kind() != post.AlbumAvatars {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	second, _ := post.NewAvatarAlbum(ann, time.Now())
	if err = albums.Save(ctx, second); !errors.Is(err, post.ErrAvatarAlbumExists) {
		t.Fatalf("a second one: %v", err)
	}

	// Any number of albums people make.
	for range 2 {
		own, _ := post.NewAlbum(ann, "Лето", "", time.Now())
		if err = albums.Save(ctx, own); err != nil {
			t.Fatalf("an album of ann's own: %v", err)
		}
	}
}

func TestAlbumPhotoReactions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	albums := postgres.NewAlbumRepository(pool)
	likes, comments := albums.PhotoLikes(), albums.PhotoComments()
	posts := postgres.NewPostRepository(pool)
	ann, bob := uuid.New(), uuid.New()

	a, _ := post.NewAlbum(ann, "Лето", "", time.Now())
	if err := albums.Save(ctx, a); err != nil {
		t.Fatal(err)
	}
	photo := newAlbumPhotos(t, a, ann, 1, time.Now())[0]
	if err := albums.Photos().Add(ctx, a.ID(), []*post.AlbumPhoto{photo}, time.Now()); err != nil {
		t.Fatal(err)
	}

	if n, err := likes.Like(ctx, photo.ID(), bob); err != nil || n != 1 {
		t.Fatalf("Like = %d, %v", n, err)
	}
	if n, err := likes.Like(ctx, photo.ID(), bob); err != nil || n != 1 {
		t.Fatalf("a second Like = %d, %v", n, err)
	}
	if _, err := likes.Like(ctx, uuid.New(), bob); !errors.Is(err, post.ErrAlbumPhotoNotFound) {
		t.Fatalf("liking no photo: %v", err)
	}
	if liked, err := likes.Liked(ctx, bob, []uuid.UUID{photo.ID()}); err != nil || !liked[photo.ID()] {
		t.Fatalf("Liked = %v, %v", liked, err)
	}
	if found, err := likes.Likers(ctx, photo.ID(), nil, 10); err != nil || len(found) != 1 || found[0].UserID != bob {
		t.Fatalf("Likers = %+v, %v", found, err)
	}

	c, _ := post.NewPhotoComment(photo.ID(), bob, "красиво", time.Now())
	if err := comments.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	missing, _ := post.NewPhotoComment(uuid.New(), bob, "x", time.Now())
	if err := comments.Save(ctx, missing); !errors.Is(err, post.ErrAlbumPhotoNotFound) {
		t.Fatalf("a comment on no photo: %v", err)
	}
	if got, err := comments.Get(ctx, c.ID()); err != nil || got.Text() != "красиво" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if got, err := albums.Photos().Get(ctx, photo.ID()); err != nil || got.Likes() != 1 || got.Comments() != 1 {
		t.Fatalf("counters = %+v, %v", got, err)
	}

	// A post sharing the photo keeps its ID.
	content, _ := post.NewPhotoShare(photo.ID(), "смотрите")
	share, _ := post.New(bob, bob, content, time.Now())
	if err := posts.Save(ctx, share); err != nil {
		t.Fatalf("Save a share: %v", err)
	}
	if got, err := posts.Get(ctx, share.ID()); err != nil || got.Content().SharedPhoto() != photo.ID() {
		t.Fatalf("Get a share = %+v, %v", got, err)
	}

	// Erasing bob takes his like and comment, uncounted.
	if err := albums.DeleteByUser(ctx, bob); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if got, err := albums.Photos().Get(ctx, photo.ID()); err != nil || got.Likes() != 0 || got.Comments() != 0 {
		t.Fatalf("after erasing bob = %+v, %v", got, err)
	}
	if _, err := comments.Get(ctx, c.ID()); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("bob's comment: %v", err)
	}
}
