package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestTopicRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	topics := postgres.NewTopicRepository(newPool(t))
	messages := topics.Messages()
	club, ann, bob := uuid.New(), uuid.New(), uuid.New()
	start := time.Now().Add(-time.Hour)

	hello, err := post.NewTopic(club, ann, "Знакомство", false, start)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := post.NewTopic(club, bob, "Правила группы", true, start.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, topic := range []*post.Topic{hello, rules} {
		if err = topics.Save(ctx, topic); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	if got, err := topics.Get(ctx, rules.ID()); err != nil || got.Stored() != rules.Stored() {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, rules.Stored())
	}

	// A message counts in its topic and makes it the most recent.
	photo := uuid.New()
	content, _ := post.NewCommentContent("Привет", post.Photos{photo})
	hi, err := post.NewTopicMessage(hello, bob, content, false, start.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, hi); err != nil {
		t.Fatalf("Save message: %v", err)
	}
	if got, _ := topics.Get(ctx, hello.ID()); got.Messages() != 1 || !got.UpdatedAt().Equal(hi.CreatedAt()) {
		t.Fatalf("after a message = %+v", got.Stored())
	}
	if got, err := messages.Get(ctx, hi.ID()); err != nil || got.Content().Photos()[0] != photo || got.AuthorID() != bob {
		t.Fatalf("Get message = %+v, %v", got, err)
	}

	list, err := topics.List(ctx, club, "", nil, 10)
	if err != nil || len(list) != 2 || list[0].ID() != hello.ID() {
		t.Fatalf("List = %v, %v; the active one first", list, err)
	}

	// Pinned first, then a page after the cursor.
	pinned, _ := rules.Changed(bob, true, post.TopicChanges{Title: "Правила", Pinned: true, Restricted: true})
	if err = topics.Update(ctx, pinned); err != nil {
		t.Fatal(err)
	}
	first, err := topics.List(ctx, club, "", nil, 1)
	if err != nil || len(first) != 1 || first[0].ID() != rules.ID() || !first[0].Pinned() || !first[0].Restricted() || first[0].Title() != "Правила" {
		t.Fatalf("first page = %v, %v", first, err)
	}
	after := &service.TopicCursor{Pinned: first[0].Pinned(), UpdatedAt: first[0].UpdatedAt(), ID: first[0].ID()}
	if next, err := topics.List(ctx, club, "", after, 10); err != nil || len(next) != 1 || next[0].ID() != hello.ID() {
		t.Fatalf("next page = %v, %v", next, err)
	}

	if found, err := topics.List(ctx, club, "ЗНАКОМ", nil, 10); err != nil || len(found) != 1 || found[0].ID() != hello.ID() {
		t.Fatalf("search = %v, %v", found, err)
	}
	if n, err := topics.Count(ctx, club, "правила"); err != nil || n != 1 {
		t.Fatalf("Count = %d, %v", n, err)
	}

	second, _ := post.NewTopicMessage(hello, ann, content, false, start.Add(3*time.Minute))
	if err = messages.Save(ctx, second); err != nil {
		t.Fatal(err)
	}
	if page, err := messages.List(ctx, hello.ID(), hi.ID(), 10); err != nil || len(page) != 1 || page[0].ID() != second.ID() {
		t.Fatalf("messages after the first = %v, %v", page, err)
	}
	if last, err := messages.Last(ctx, []uuid.UUID{hello.ID(), rules.ID()}); err != nil || len(last) != 1 || last[0].ID() != second.ID() {
		t.Fatalf("Last = %v, %v", last, err)
	}

	// Hearts: one per user, counted in the message.
	for range 2 {
		if likes, err := messages.Like(ctx, second.ID(), bob); err != nil || likes != 1 {
			t.Fatalf("Like = %d, %v", likes, err)
		}
	}
	if likes, err := messages.Like(ctx, second.ID(), ann); err != nil || likes != 2 {
		t.Fatalf("a second Like = %d, %v", likes, err)
	}
	if _, err = messages.Like(ctx, uuid.New(), bob); !errors.Is(err, post.ErrMessageNotFound) {
		t.Fatalf("Like of nothing: %v", err)
	}
	if _, err = messages.Unlike(ctx, uuid.New(), bob); !errors.Is(err, post.ErrMessageNotFound) {
		t.Fatalf("Unlike of nothing: %v", err)
	}
	if liked, err := messages.Liked(ctx, bob, []uuid.UUID{hi.ID(), second.ID()}); err != nil || len(liked) != 1 || !liked[second.ID()] {
		t.Fatalf("Liked = %v, %v", liked, err)
	}
	if got, err := messages.Get(ctx, second.ID()); err != nil || got.Likes() != 2 {
		t.Fatalf("Get = %d likes, %v", got.Likes(), err)
	}
	for range 2 {
		if likes, err := messages.Unlike(ctx, second.ID(), ann); err != nil || likes != 1 {
			t.Fatalf("Unlike = %d, %v", likes, err)
		}
	}
	// An erased user takes their hearts out of the count.
	if _, err = topics.DeleteByUser(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if page, err := messages.List(ctx, hello.ID(), hi.ID(), 10); err != nil || page[0].Likes() != 1 {
		t.Fatalf("likes after erasing a stranger = %+v, %v", page, err)
	}

	if err = messages.Delete(ctx, second.ID()); err != nil {
		t.Fatal(err)
	}
	if got, _ := topics.Get(ctx, hello.ID()); got.Messages() != 1 {
		t.Fatalf("after deleting a message = %+v", got.Stored())
	}
	if err = messages.Delete(ctx, second.ID()); !errors.Is(err, post.ErrMessageNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}

	// A deleted topic takes its messages; a message for it is not saved.
	gone, err := topics.Delete(ctx, hello.ID())
	if err != nil || len(gone) != 1 || gone[0].ID() != hi.ID() {
		t.Fatalf("Delete = %v, %v", gone, err)
	}
	if _, err = topics.Delete(ctx, hello.ID()); !errors.Is(err, post.ErrTopicNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}
	late, _ := post.NewTopicMessage(hello, bob, content, false, time.Now())
	if err = messages.Save(ctx, late); !errors.Is(err, post.ErrTopicNotFound) {
		t.Fatalf("a message in a deleted topic: %v", err)
	}
}

func TestTopicRepository_DeleteByUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	topics := postgres.NewTopicRepository(newPool(t))
	messages := topics.Messages()
	club, ann, bob := uuid.New(), uuid.New(), uuid.New()
	content, _ := post.NewCommentContent("текст", nil)

	annsTopic, _ := post.NewTopic(club, ann, "Анна", false, time.Now())
	bobsTopic, _ := post.NewTopic(club, bob, "Пётр", false, time.Now())
	for _, topic := range []*post.Topic{annsTopic, bobsTopic} {
		if err := topics.Save(ctx, topic); err != nil {
			t.Fatal(err)
		}
	}
	annInHers, _ := post.NewTopicMessage(annsTopic, ann, content, false, time.Now())
	bobInHers, _ := post.NewTopicMessage(annsTopic, bob, content, false, time.Now())
	annInHis, _ := post.NewTopicMessage(bobsTopic, ann, content, false, time.Now())
	for _, m := range []*post.TopicMessage{annInHers, bobInHers, annInHis} {
		if err := messages.Save(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	gone, err := topics.DeleteByUser(ctx, ann)
	if err != nil || len(gone) != 1 || gone[0].ID() != bobInHers.ID() {
		t.Fatalf("DeleteByUser = %v, %v; bob's message in ann's topic", gone, err)
	}
	if _, err = topics.Get(ctx, annsTopic.ID()); !errors.Is(err, post.ErrTopicNotFound) {
		t.Fatalf("ann's topic: %v", err)
	}
	if got, err := topics.Get(ctx, bobsTopic.ID()); err != nil || got.Messages() != 0 {
		t.Fatalf("bob's topic = %+v, %v; ann's message uncounted", got, err)
	}
}
