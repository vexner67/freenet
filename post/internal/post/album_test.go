package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewAlbumName(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in, want string
		err      error
	}{
		{"  Лето  ", "Лето", nil},
		{"Лето\n2026\tгод", "Лето 2026 год", nil},
		{" \n ", "", post.ErrAlbumNameEmpty},
		{strings.Repeat("я", 36), strings.Repeat("я", 36), nil},
		{strings.Repeat("я", 37), "", post.ErrAlbumNameTooLong},
	} {
		got, err := post.NewAlbumName(tt.in)
		if !errors.Is(err, tt.err) || got.String() != tt.want {
			t.Errorf("NewAlbumName(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
		}
	}
}

func TestNewDescription(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in, want string
		err      error
	}{
		{"", "", nil},
		{"  На даче\r\nс котом  ", "На даче\nс котом", nil},
		{strings.Repeat("я", 255), strings.Repeat("я", 255), nil},
		{strings.Repeat("я", 256), "", post.ErrDescriptionTooLong},
	} {
		got, err := post.NewDescription(tt.in)
		if !errors.Is(err, tt.err) || got.String() != tt.want {
			t.Errorf("NewDescription(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
		}
	}
}

func TestAlbum(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	now := time.Now()

	a, err := post.NewAlbum(ann, "Лето", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if a.OwnerID() != ann || a.Photos() != 0 || !a.UpdatedAt().Equal(a.CreatedAt()) {
		t.Fatalf("album = %+v", a)
	}

	if !a.CanBeChangedBy(ann, false) || a.CanBeChangedBy(bob, false) || !a.CanBeChangedBy(bob, true) {
		t.Fatal("CanBeChangedBy: the owner or an admin of the community")
	}

	later := now.Add(time.Hour)
	renamed := a.Renamed("Осень", "Листья", later)
	if renamed.Name() != "Осень" || renamed.Description() != "Листья" || !renamed.UpdatedAt().Equal(later.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("Renamed = %+v", renamed)
	}
	if a.Name() != "Лето" {
		t.Fatal("Renamed changed the album in place")
	}

	restored := post.RestoreAlbum(post.StoredAlbum{ID: a.ID(), OwnerID: ann, Name: "Лето", CreatedAt: a.CreatedAt(), UpdatedAt: later, Photos: 3})
	if restored.Photos() != 3 || restored.ID() != a.ID() {
		t.Fatalf("RestoreAlbum = %+v", restored)
	}
}

func TestAlbumPhoto(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	a, err := post.NewAlbum(ann, "Лето", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if _, err = post.NewAlbumPhoto(a, uuid.Nil, ann, "", time.Now()); !errors.Is(err, post.ErrInvalidPhoto) {
		t.Fatalf("no photo: %v", err)
	}

	photoID := uuid.Must(uuid.NewV7())
	p, err := post.NewAlbumPhoto(a, photoID, ann, "Море", time.Now())
	if err != nil || p.ID() != photoID || p.AlbumID() != a.ID() || p.AuthorID() != ann {
		t.Fatalf("NewAlbumPhoto = %+v, %v", p, err)
	}

	described, err := p.Described(ann, "Море и кот")
	if err != nil || described.Description() != "Море и кот" || p.Description() != "Море" {
		t.Fatalf("Described = %+v, %v", described, err)
	}
	if _, err = p.Described(bob, "x"); !errors.Is(err, post.ErrCannotDescribePhoto) {
		t.Fatalf("a stranger describes: %v", err)
	}

	if !p.CanBeDeletedBy(ann, false) || p.CanBeDeletedBy(bob, false) || !p.CanBeDeletedBy(bob, true) {
		t.Fatal("CanBeDeletedBy: who uploaded it or the owner of the album")
	}
}
