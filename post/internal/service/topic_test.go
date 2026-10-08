package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

func newTopicService(t *testing.T, moderators fakeModerators) (*service.TopicService, *fakePhotos) {
	t.Helper()

	topics := inmem.NewTopicRepository()
	photos := &fakePhotos{}

	return service.NewTopicService(service.TopicDeps{
		Topics: topics, Messages: topics.Messages(), Tx: fakeTx{}, Photos: photos, Moderators: moderators,
	}), photos
}

func words(t *testing.T, text string, photos ...uuid.UUID) *post.CommentContent {
	t.Helper()

	c, err := post.NewCommentContent(post.CommentText(text), photos)
	if err != nil {
		t.Fatal(err)
	}

	return &c
}

func TestTopicService_StartAndList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, ann := uuid.New(), uuid.New(), uuid.New()
	svc, photos := newTopicService(t, fakeModerators{{club, admin}: true})

	photo := uuid.New()
	hello, err := svc.Start(ctx, service.StartTopicInput{ClubID: club, AuthorID: ann, Title: "Знакомство", Content: words(t, "Привет!", photo)})
	if err != nil {
		t.Fatal(err)
	}
	if hello.Messages() != 1 || len(photos.published) != 1 || photos.published[0].owner != ann || photos.outsideTx != 0 {
		t.Fatalf("topic = %+v, photos %+v; the first message and its photo", hello.Stored(), photos.published)
	}

	empty, err := svc.Start(ctx, service.StartTopicInput{ClubID: club, AuthorID: admin, Title: "Правила", FromWall: true})
	if err != nil || empty.Messages() != 0 || !empty.FromWall() {
		t.Fatalf("a topic without a message = %+v, %v", empty, err)
	}

	// The most recently active first: a message moves a topic up.
	if _, err = svc.Write(ctx, service.TopicMessageInput{TopicID: hello.ID(), AuthorID: admin, Content: *words(t, "Добро пожаловать")}); err != nil {
		t.Fatal(err)
	}
	page, err := svc.List(ctx, club, "", nil, 10)
	if err != nil || len(page.Topics) != 2 || page.Topics[0].ID() != hello.ID() || page.Total != 2 || page.NextCursor != nil {
		t.Fatalf("list = %+v, %v", page, err)
	}
	if last := page.Last[hello.ID()]; last == nil || last.AuthorID() != admin || page.Last[empty.ID()] != nil {
		t.Fatalf("the newest messages = %+v", page.Last)
	}

	// A pinned topic comes first; pages go on after the cursor.
	if _, err = svc.Change(ctx, admin, empty.ID(), post.TopicChanges{Title: "Правила", Pinned: true}); err != nil {
		t.Fatal(err)
	}
	first, err := svc.List(ctx, club, "", nil, 1)
	if err != nil || len(first.Topics) != 1 || first.Topics[0].ID() != empty.ID() || first.NextCursor == nil {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	second, err := svc.List(ctx, club, "", first.NextCursor, 1)
	if err != nil || len(second.Topics) != 1 || second.Topics[0].ID() != hello.ID() || second.NextCursor != nil {
		t.Fatalf("second page = %+v, %v", second, err)
	}

	found, err := svc.List(ctx, club, "знаком", nil, 10)
	if err != nil || len(found.Topics) != 1 || found.Topics[0].ID() != hello.ID() || found.Total != 1 {
		t.Fatalf("search = %+v, %v", found, err)
	}
	if other, _ := svc.List(ctx, uuid.New(), "", nil, 10); len(other.Topics) != 0 {
		t.Fatalf("another club's = %+v", other.Topics)
	}
}

func TestTopicService_Rights(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, ann, bob := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	svc, _ := newTopicService(t, fakeModerators{{club, admin}: true})

	topic, err := svc.Start(ctx, service.StartTopicInput{ClubID: club, AuthorID: ann, Title: "Знакомство"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = svc.Change(ctx, bob, topic.ID(), post.TopicChanges{Title: "Моя"}); !errors.Is(err, post.ErrCannotChangeTopic) {
		t.Fatalf("bob renames: %v", err)
	}
	if _, err = svc.Change(ctx, ann, topic.ID(), post.TopicChanges{Title: "Знакомство", Pinned: true}); !errors.Is(err, post.ErrCannotPinTopic) {
		t.Fatalf("ann pins: %v", err)
	}
	closed, err := svc.Change(ctx, ann, topic.ID(), post.TopicChanges{Title: "Знакомимся", Closed: true})
	if err != nil || closed.Title() != "Знакомимся" || !closed.Closed() {
		t.Fatalf("ann closes = %+v, %v", closed, err)
	}
	if got, _ := svc.Get(ctx, topic.ID()); !got.Closed() || got.Title() != "Знакомимся" {
		t.Fatalf("stored = %+v", got.Stored())
	}

	// Nobody writes in a closed topic, the admins neither.
	for _, who := range []uuid.UUID{bob, admin} {
		if _, err = svc.Write(ctx, service.TopicMessageInput{TopicID: topic.ID(), AuthorID: who, Content: *words(t, "а я?")}); !errors.Is(err, post.ErrTopicClosed) {
			t.Fatalf("writes in a closed topic: %v", err)
		}
	}

	// Only the admins write in a restricted topic of the community.
	news, _ := svc.Start(ctx, service.StartTopicInput{ClubID: club, AuthorID: admin, Title: "Новости", FromWall: true})
	if _, err = svc.Change(ctx, admin, news.ID(), post.TopicChanges{Title: "Новости", Restricted: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Write(ctx, service.TopicMessageInput{TopicID: news.ID(), AuthorID: bob, Content: *words(t, "а я?")}); !errors.Is(err, post.ErrTopicRestricted) {
		t.Fatalf("bob writes in a restricted topic: %v", err)
	}
	if _, err = svc.Write(ctx, service.TopicMessageInput{TopicID: news.ID(), AuthorID: admin, Content: *words(t, "Новость"), FromWall: true}); err != nil {
		t.Fatalf("the admin writes: %v", err)
	}

	if err = svc.Delete(ctx, bob, topic.ID()); !errors.Is(err, post.ErrCannotChangeTopic) {
		t.Fatalf("bob deletes: %v", err)
	}
	if err = svc.Delete(ctx, admin, topic.ID()); err != nil {
		t.Fatalf("the admin deletes: %v", err)
	}
	if _, err = svc.Get(ctx, topic.ID()); !errors.Is(err, post.ErrTopicNotFound) {
		t.Fatalf("deleted: %v", err)
	}
	if _, err = svc.Write(ctx, service.TopicMessageInput{TopicID: topic.ID(), AuthorID: bob, Content: *words(t, "эй")}); !errors.Is(err, post.ErrTopicNotFound) {
		t.Fatalf("writes in a deleted topic: %v", err)
	}
}

func TestTopicService_Messages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	club, admin, ann, bob := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	svc, photos := newTopicService(t, fakeModerators{{club, admin}: true})

	topic, err := svc.Start(ctx, service.StartTopicInput{ClubID: club, AuthorID: ann, Title: "Знакомство", Content: words(t, "Привет!")})
	if err != nil {
		t.Fatal(err)
	}

	photo := uuid.New()
	bobs, err := svc.Write(ctx, service.TopicMessageInput{TopicID: topic.ID(), AuthorID: bob, Content: *words(t, "", photo)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Write(ctx, service.TopicMessageInput{TopicID: topic.ID(), AuthorID: ann, Content: *words(t, "Красиво")}); err != nil {
		t.Fatal(err)
	}

	first, err := svc.Messages(ctx, topic.ID(), uuid.Nil, 2)
	if err != nil || len(first.Messages) != 2 || first.Total != 3 || first.Messages[1].ID() != bobs.ID() || first.NextCursor != bobs.ID() {
		t.Fatalf("first page = %+v, %v; oldest first", first, err)
	}
	rest, err := svc.Messages(ctx, topic.ID(), first.NextCursor, 2)
	if err != nil || len(rest.Messages) != 1 || rest.NextCursor != uuid.Nil {
		t.Fatalf("rest = %+v, %v", rest, err)
	}

	// The topic's author deletes nobody's but their own; the admins any.
	if err = svc.DeleteMessage(ctx, ann, bobs.ID()); !errors.Is(err, post.ErrCannotDeleteMessage) {
		t.Fatalf("ann deletes bob's: %v", err)
	}
	if err = svc.DeleteMessage(ctx, admin, bobs.ID()); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != bob || photos.deleted[0].photos[0] != photo {
		t.Fatalf("the photo goes with it: %+v", photos.deleted)
	}
	if got, _ := svc.Get(ctx, topic.ID()); got.Messages() != 2 {
		t.Fatalf("messages = %d after deleting one", got.Messages())
	}
	if err = svc.DeleteMessage(ctx, admin, bobs.ID()); !errors.Is(err, post.ErrMessageNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}

	// A deleted topic takes the photos of its messages.
	last, _ := svc.Write(ctx, service.TopicMessageInput{TopicID: topic.ID(), AuthorID: bob, Content: *words(t, "", uuid.New())})
	if err = svc.Delete(ctx, ann, topic.ID()); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 2 || photos.deleted[1].owner != bob || photos.deleted[1].photos[0] != last.Content().Photos()[0] {
		t.Fatalf("deleted with the topic: %+v", photos.deleted)
	}
}
