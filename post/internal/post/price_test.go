package post_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewPrice(t *testing.T) {
	t.Parallel()

	for _, votes := range []int64{0, 1, post.MaxPrice} {
		if _, err := post.NewPrice(votes); err != nil {
			t.Errorf("NewPrice(%d) = %v", votes, err)
		}
	}
	for _, votes := range []int64{-1, post.MaxPrice + 1} {
		if _, err := post.NewPrice(votes); !errors.Is(err, post.ErrInvalidPrice) {
			t.Errorf("NewPrice(%d) = %v, want ErrInvalidPrice", votes, err)
		}
	}
}

func TestPost_Priced(t *testing.T) {
	t.Parallel()

	ann, bob, club := uuid.New(), uuid.New(), uuid.New()
	photos, _ := post.NewPhotos([]uuid.UUID{uuid.New()})
	withPhotos, _ := post.NewContent("", photos)
	textOnly, _ := post.NewContent("hi", nil)
	now := time.Now()

	own, _ := post.New(ann, ann, withPhotos, now)
	priced, err := own.Priced(50)
	if err != nil {
		t.Fatalf("Priced = %v", err)
	}
	if priced.Price() != 50 || own.Price() != 0 {
		t.Errorf("price = %d, original %d", priced.Price(), own.Price())
	}
	if priced.InWallAlbum() {
		t.Error("a paid post must not put its photos in the album")
	}

	if free, err := own.Priced(0); err != nil || free.Price() != 0 {
		t.Errorf("Priced(0) = %v, %v", free, err)
	}

	guest, _ := post.New(bob, ann, withPhotos, now)
	clubs, _ := post.NewFromWall(club, ann, withPhotos, false, now)
	for name, p := range map[string]*post.Post{"on another's wall": guest, "for a community": clubs} {
		if _, err := p.Priced(10); !errors.Is(err, post.ErrPricedElsewhere) {
			t.Errorf("%s: Priced = %v, want ErrPricedElsewhere", name, err)
		}
	}

	plain, _ := post.New(ann, ann, textOnly, now)
	if _, err := plain.Priced(10); !errors.Is(err, post.ErrPriceWithoutMedia) {
		t.Errorf("text only: Priced = %v, want ErrPriceWithoutMedia", err)
	}

	if _, err := priced.Edited(ann, "now words", nil, now); !errors.Is(err, post.ErrPriceWithoutMedia) {
		t.Errorf("edit taking the photos away = %v, want ErrPriceWithoutMedia", err)
	}
	if edited, err := priced.Edited(ann, "now words", photos, now); err != nil || edited.Price() != 50 {
		t.Errorf("edit keeping the photos = %v, %v", edited, err)
	}
}
