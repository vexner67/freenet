package post_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestPhotoComment(t *testing.T) {
	t.Parallel()

	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	album, _ := post.NewAlbum(ann, "Лето", "", time.Now())
	photo, _ := post.NewAlbumPhoto(album, uuid.Must(uuid.NewV7()), bob, "", time.Now())

	if _, err := post.NewPhotoComment(photo.ID(), cat, "", time.Now()); !errors.Is(err, post.ErrCommentEmpty) {
		t.Fatalf("an empty comment: %v", err)
	}

	c, err := post.NewPhotoComment(photo.ID(), cat, "красиво", time.Now())
	if err != nil || c.PhotoID() != photo.ID() || c.AuthorID() != cat {
		t.Fatalf("NewPhotoComment = %+v, %v", c, err)
	}

	for _, tt := range []struct {
		name         string
		who          uuid.UUID
		changesAlbum bool
		want         bool
	}{
		{"its author", cat, false, true},
		{"who uploaded the photo", bob, false, true},
		{"the owner of the album", ann, true, true},
		{"anyone else", uuid.New(), false, false},
	} {
		if got := c.CanBeDeletedBy(tt.who, photo, tt.changesAlbum); got != tt.want {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
}

func TestPhotoReply(t *testing.T) {
	t.Parallel()

	photo, other, ann, bob := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	parent, err := post.NewPhotoComment(photo, ann, "красиво", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	reply, err := post.NewPhotoReply(photo, parent, bob, "да", time.Now())
	if err != nil || reply.ReplyTo() != parent.ID() || reply.PhotoID() != photo || reply.Likes() != 0 {
		t.Fatalf("NewPhotoReply = %+v, %v", reply, err)
	}

	if _, err = post.NewPhotoReply(other, parent, bob, "да", time.Now()); !errors.Is(err, post.ErrReplyOtherPhoto) {
		t.Fatalf("a reply under another photo: %v", err)
	}
	if _, err = post.NewPhotoReply(photo, parent, bob, "", time.Now()); !errors.Is(err, post.ErrCommentEmpty) {
		t.Fatalf("an empty reply: %v", err)
	}

	// Counting hearts and losing the comment answered leave the reply as
	// it was handed out.
	if liked := reply.WithLikes(3); liked.Likes() != 3 || reply.Likes() != 0 || liked.ReplyTo() != parent.ID() {
		t.Fatalf("WithLikes = %+v", liked)
	}
	if alone := reply.WithoutReply(); alone.ReplyTo() != uuid.Nil || reply.ReplyTo() != parent.ID() {
		t.Fatalf("WithoutReply = %+v", alone)
	}
}

func TestPhotoShare(t *testing.T) {
	t.Parallel()

	ann, photo := uuid.New(), uuid.New()

	if _, err := post.NewPhotoShare(uuid.Nil, ""); !errors.Is(err, post.ErrInvalidPhoto) {
		t.Fatalf("no photo: %v", err)
	}

	content, err := post.NewPhotoShare(photo, "смотрите")
	if err != nil || content.SharedPhoto() != photo || content.RepostOf() != uuid.Nil {
		t.Fatalf("NewPhotoShare = %+v, %v", content, err)
	}

	p, err := post.New(ann, ann, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Edited, it shares the photo still and takes no photos of its own.
	edited, err := p.Edited(ann, "лучше так", nil, time.Now())
	if err != nil || edited.Content().SharedPhoto() != photo || edited.Content().Text() != "лучше так" {
		t.Fatalf("Edited = %+v, %v", edited, err)
	}
	if _, err = p.Edited(ann, "", post.Photos{uuid.New()}, time.Now()); !errors.Is(err, post.ErrRepostPhotos) {
		t.Fatalf("photos on a share: %v", err)
	}
}
