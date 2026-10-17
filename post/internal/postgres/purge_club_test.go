package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

// A community that is gone takes what was suggested and postponed for its
// wall, its topics and its albums; those of other walls stay.
func TestDeleteByClub(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	suggestions := postgres.NewSuggestionRepository(pool)
	postponed := postgres.NewPostponedRepository(pool)
	topics := postgres.NewTopicRepository(pool)
	albums := postgres.NewAlbumRepository(pool)

	club, other, ann := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()
	content := post.RestoreContent("встреча", post.Photos{uuid.New()}, uuid.Nil)

	for _, wall := range []uuid.UUID{club, other} {
		s, err := post.NewSuggestion(wall, ann, content, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = suggestions.Save(ctx, s); err != nil {
			t.Fatal(err)
		}

		p, err := post.NewPostponed(post.PostponedDraft{WallID: wall, AuthorID: ann, Content: content, FromWall: true, PublishAt: now.Add(time.Hour)}, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = postponed.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	title, err := post.NewTopicTitle("Кто придёт?")
	if err != nil {
		t.Fatal(err)
	}
	topic, err := post.NewTopic(club, ann, title, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = topics.Save(ctx, topic); err != nil {
		t.Fatal(err)
	}
	message, err := post.NewTopicMessage(topic, ann, post.RestoreCommentContent("я", nil), false, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = topics.Messages().Save(ctx, message); err != nil {
		t.Fatal(err)
	}

	name, err := post.NewAlbumName("Фото")
	if err != nil {
		t.Fatal(err)
	}
	album, err := post.NewAlbum(club, name, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = albums.Save(ctx, album); err != nil {
		t.Fatal(err)
	}
	photo, err := post.NewAlbumPhoto(album, uuid.New(), ann, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = albums.Photos().Add(ctx, album.ID(), []*post.AlbumPhoto{photo}, now); err != nil {
		t.Fatal(err)
	}

	gone, err := suggestions.DeleteOnWall(ctx, club)
	if err != nil || len(gone) != 1 || gone[0].WallID() != club || len(gone[0].Content().Photos()) != 1 {
		t.Fatalf("DeleteOnWall suggestions = %v, %v", gone, err)
	}
	waiting, err := postponed.DeleteOnWall(ctx, club)
	if err != nil || len(waiting) != 1 || waiting[0].WallID() != club {
		t.Fatalf("DeleteOnWall postponed = %v, %v", waiting, err)
	}
	ids, messages, err := topics.DeleteByClub(ctx, club)
	if err != nil || len(ids) != 1 || ids[0] != topic.ID() || len(messages) != 1 || messages[0].ID() != message.ID() {
		t.Fatalf("DeleteByClub = %v, %v, %v", ids, messages, err)
	}
	photos, err := albums.DeleteByOwner(ctx, club)
	if err != nil || len(photos) != 1 || photos[0].ID() != photo.ID() {
		t.Fatalf("DeleteByOwner = %v, %v", photos, err)
	}

	if n, _ := suggestions.Count(ctx, other, uuid.Nil); n != 1 {
		t.Errorf("suggestions of another wall: %d, want 1", n)
	}
	if _, err = topics.Get(ctx, topic.ID()); !errors.Is(err, post.ErrTopicNotFound) {
		t.Errorf("the topic is still there: %v", err)
	}
	if _, err = albums.Get(ctx, album.ID()); !errors.Is(err, post.ErrAlbumNotFound) {
		t.Errorf("the album is still there: %v", err)
	}

	// Nothing is left to delete the second time.
	if ids, messages, err = topics.DeleteByClub(ctx, club); err != nil || len(ids) != 0 || len(messages) != 0 {
		t.Errorf("DeleteByClub twice = %v, %v, %v", ids, messages, err)
	}
}
