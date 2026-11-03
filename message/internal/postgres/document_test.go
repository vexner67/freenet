package postgres_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/postgres"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// Integration test: needs a database with migrations applied.
func TestDocuments(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	messages := postgres.NewMessageRepository(pool)
	dialogs := postgres.NewDialogRepository(pool)
	ann, bob := uuid.New(), uuid.New()
	docs := message.Documents{uuid.New(), uuid.New()}

	content, err := message.NewAttachedContent(docs, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := message.New(ann, bob, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, m); err != nil {
		t.Fatalf("Save documents: %v", err)
	}

	if got, err := messages.Get(ctx, m.ID()); err != nil || !slices.Equal(got.Documents(), docs) || got.Sealed() != "" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if list, err := dialogs.List(ctx, bob, uuid.Nil, 1); err != nil || len(list) != 1 || !slices.Equal(list[0].LastMessage.Documents(), docs) {
		t.Fatalf("the dialog ends with %+v, %v", list, err)
	}

	forward, err := message.New(bob, ann, said("", nil, uuid.Nil, m.Carried()), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, forward); err != nil {
		t.Fatal(err)
	}
	if got, err := messages.Get(ctx, forward.ID()); err != nil || len(got.Forwarded()) != 1 || !slices.Equal(got.Forwarded()[0].Documents, docs) {
		t.Fatalf("forwarded documents = %+v, %v", got, err)
	}

	gone, err := m.TakenBack(ann, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Retract(ctx, gone); err != nil {
		t.Fatalf("Retract documents: %v", err)
	}
	if got, err := messages.Get(ctx, m.ID()); err != nil || len(got.Documents()) != 0 || !got.IsDeleted() {
		t.Fatalf("taken back = %+v, %v", got, err)
	}
}

// Integration test: needs a database with migrations applied.
func TestChats_Documents(t *testing.T) {
	svc, ctx := chatService(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	docs := message.Documents{uuid.New()}

	view, err := svc.Create(ctx, ann, "Горы", []uuid.UUID{bob, cat})
	if err != nil {
		t.Fatal(err)
	}
	chatID := view.Chat.ID()

	sent, err := svc.Send(ctx, bob, chatID, service.Draft{Documents: docs})
	if err != nil {
		t.Fatalf("Send documents: %v", err)
	}

	if got, _, err := svc.Message(ctx, cat, sent.ID()); err != nil || !slices.Equal(got.Documents(), docs) || got.Text() != "" {
		t.Fatalf("documents read back = %+v, %v", got, err)
	}
	if page, err := svc.Chats(ctx, cat, uuid.Nil, 10); err != nil || len(page.Chats) != 1 || !slices.Equal(page.Chats[0].LastMessage.Documents(), docs) {
		t.Fatalf("the chat ends with %+v, %v", page, err)
	}

	if err = svc.Delete(ctx, bob, sent.ID()); err != nil {
		t.Fatalf("take documents back: %v", err)
	}
	if got, _, err := svc.Message(ctx, cat, sent.ID()); err != nil || len(got.Documents()) != 0 || !got.IsDeleted() {
		t.Fatalf("taken back = %+v, %v", got, err)
	}
}

// Integration test: needs a database with migrations applied.
func TestAudios(t *testing.T) {
	svc, ctx := chatService(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	songs := message.Audios{uuid.New(), uuid.New()}

	view, err := svc.Create(ctx, ann, "Музыка", []uuid.UUID{bob, cat})
	if err != nil {
		t.Fatal(err)
	}
	chatID := view.Chat.ID()

	sent, err := svc.Send(ctx, bob, chatID, service.Draft{Audios: songs})
	if err != nil {
		t.Fatalf("Send songs: %v", err)
	}
	if got, _, err := svc.Message(ctx, cat, sent.ID()); err != nil || !slices.Equal(got.Audios(), songs) {
		t.Fatalf("songs read back = %+v, %v", got, err)
	}
	if page, err := svc.Chats(ctx, cat, uuid.Nil, 10); err != nil || len(page.Chats) != 1 || !slices.Equal(page.Chats[0].LastMessage.Audios(), songs) {
		t.Fatalf("the chat ends with %+v, %v", page, err)
	}

	carried, err := svc.Send(ctx, cat, chatID, service.Draft{Forward: []uuid.UUID{sent.ID()}})
	if err != nil {
		t.Fatalf("forward songs: %v", err)
	}
	if got, _, err := svc.Message(ctx, ann, carried.ID()); err != nil || len(got.Forwarded()) != 1 || !slices.Equal(got.Forwarded()[0].Audios, songs) {
		t.Fatalf("forwarded songs = %+v, %v", got, err)
	}

	if err = svc.Delete(ctx, bob, sent.ID()); err != nil {
		t.Fatalf("take songs back: %v", err)
	}
	if got, _, err := svc.Message(ctx, cat, sent.ID()); err != nil || len(got.Audios()) != 0 || !got.IsDeleted() {
		t.Fatalf("taken back = %+v, %v", got, err)
	}

	// A dialog keeps them too.
	pool, err := pgpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	messages := postgres.NewMessageRepository(pool)
	content, err := message.NewAttachedContent(nil, songs)
	if err != nil {
		t.Fatal(err)
	}
	m, err := message.New(ann, bob, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, m); err != nil {
		t.Fatalf("Save songs: %v", err)
	}
	if got, err := messages.Get(ctx, m.ID()); err != nil || !slices.Equal(got.Audios(), songs) {
		t.Fatalf("Get = %+v, %v", got, err)
	}
}
