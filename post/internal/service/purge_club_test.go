package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestClubPurgeService(t *testing.T) {
	t.Parallel()

	store := inmem.NewStore()
	notifier := &fakeNotifier{}
	photos := &fakePhotos{}
	suggestions, postponed := inmem.NewSuggestionRepository(), inmem.NewPostponedRepository()
	topics, albums := inmem.NewTopicRepository(), inmem.NewAlbumRepository()
	posts := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: notifier, Photos: photos, Moderators: fakeModerators{}})
	comments := service.NewCommentService(service.CommentDeps{Posts: store.Posts(), Comments: store.Comments(), Likes: store.CommentLikes(), Tx: fakeTx{}, Notifier: notifier, Photos: photos, Moderators: fakeModerators{}})
	purge := service.NewClubPurgeService(service.ClubPurgeDeps{
		Posts: store.Posts(), Comments: store.Comments(), Suggestions: suggestions, Postponed: postponed,
		Topics: topics, Albums: albums, Sources: store.Sources(), Tx: fakeTx{}, Notifier: notifier, Media: photos,
	})
	ctx := context.Background()
	now := time.Now()
	club, other, ann, bob := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	// Ann writes on the wall of the club for it, bob comments with a photo.
	annsPhoto, bobsPhoto := uuid.New(), uuid.New()
	content, err := post.NewContent("встреча", post.Photos{annsPhoto})
	if err != nil {
		t.Fatal(err)
	}
	onWall, err := posts.Create(ctx, service.CreateInput{WallID: club, AuthorID: ann, Content: content, FromWall: true, ClubWall: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = comments.Create(ctx, service.CommentInput{AuthorID: bob, PostID: onWall.ID(), Content: post.RestoreCommentContent("", post.Photos{bobsPhoto})}); err != nil {
		t.Fatal(err)
	}
	onOther, err := posts.Create(ctx, service.CreateInput{WallID: other, AuthorID: ann, Content: content, FromWall: true, ClubWall: true})
	if err != nil {
		t.Fatal(err)
	}

	suggested, err := post.NewSuggestion(club, bob, content, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = suggestions.Save(ctx, suggested); err != nil {
		t.Fatal(err)
	}
	waiting, err := post.NewPostponed(post.PostponedDraft{WallID: club, AuthorID: ann, Content: content, FromWall: true, PublishAt: now.Add(time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = postponed.Save(ctx, waiting); err != nil {
		t.Fatal(err)
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
	bobsMessagePhoto := uuid.New()
	message, err := post.NewTopicMessage(topic, bob, post.RestoreCommentContent("я", post.Photos{bobsMessagePhoto}), false, now)
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
	albumPhoto, err := post.NewAlbumPhoto(album, uuid.New(), bob, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = albums.Photos().Add(ctx, album.ID(), []*post.AlbumPhoto{albumPhoto}, now); err != nil {
		t.Fatal(err)
	}

	notifier.forgotten = nil
	if err = purge.PurgeClub(ctx, club); err != nil {
		t.Fatal(err)
	}
	if err = purge.PurgeClub(ctx, club); err != nil {
		t.Fatalf("purge twice: %v", err)
	}

	if _, _, err = posts.Get(ctx, uuid.Nil, onWall.ID()); !errors.Is(err, post.ErrNotFound) {
		t.Errorf("the post on the wall is still there: %v", err)
	}
	if _, _, err = posts.Get(ctx, uuid.Nil, onOther.ID()); err != nil {
		t.Errorf("the post on another wall is gone: %v", err)
	}
	if n, _ := suggestions.Count(ctx, club, uuid.Nil); n != 0 {
		t.Errorf("suggestions left: %d", n)
	}
	if _, err = postponed.Get(ctx, waiting.ID()); !errors.Is(err, post.ErrPostponedNotFound) {
		t.Errorf("the postponed post is still there: %v", err)
	}
	if _, err = topics.Get(ctx, topic.ID()); !errors.Is(err, post.ErrTopicNotFound) {
		t.Errorf("the topic is still there: %v", err)
	}
	if _, err = albums.Get(ctx, album.ID()); !errors.Is(err, post.ErrAlbumNotFound) {
		t.Errorf("the album is still there: %v", err)
	}

	for _, id := range []uuid.UUID{onWall.ID(), suggested.ID(), topic.ID(), albumPhoto.ID()} {
		if !slices.Contains(notifier.forgotten, id) {
			t.Errorf("the notifications of %s were not forgotten", id)
		}
	}

	wantDeleted := map[uuid.UUID]uuid.UUID{
		annsPhoto:        ann,
		bobsPhoto:        bob,
		bobsMessagePhoto: bob,
		albumPhoto.ID():  bob,
	}
	for photo, owner := range wantDeleted {
		if !slices.ContainsFunc(photos.deleted, func(c photoCall) bool { return c.owner == owner && slices.Contains(c.photos, photo) }) {
			t.Errorf("photo %s of %s was not deleted: %+v", photo, owner, photos.deleted)
		}
	}
	if !slices.Contains(photos.purged, club) {
		t.Errorf("the files of the club were not purged: %v", photos.purged)
	}
	if notifier.outsideTx != 0 || photos.outsideTx != 0 {
		t.Errorf("recorded outside the transaction: %d notifications, %d photos", notifier.outsideTx, photos.outsideTx)
	}
}
