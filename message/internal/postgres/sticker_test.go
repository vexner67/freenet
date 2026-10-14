package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/postgres"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// Integration test: needs a database with migrations applied.
func TestStickers(t *testing.T) {
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

	content, err := message.NewStickerContent("cat_hi")
	if err != nil {
		t.Fatal(err)
	}
	m, err := message.New(ann, bob, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, m); err != nil {
		t.Fatalf("Save a sticker: %v", err)
	}

	if got, err := messages.Get(ctx, m.ID()); err != nil || got.Sticker() != "cat_hi" || got.Sealed() != "" || len(got.Photos()) != 0 {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if list, err := dialogs.List(ctx, bob, uuid.Nil, 1); err != nil || len(list) != 1 || list[0].LastMessage.Sticker() != "cat_hi" {
		t.Fatalf("the dialog ends with %+v, %v", list, err)
	}

	forward, err := message.New(bob, ann, said("", nil, uuid.Nil, m.Carried()), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, forward); err != nil {
		t.Fatal(err)
	}
	if got, err := messages.Get(ctx, forward.ID()); err != nil || len(got.Forwarded()) != 1 || got.Forwarded()[0].Sticker != "cat_hi" {
		t.Fatalf("a forwarded sticker = %+v, %v", got, err)
	}

	gone, err := m.TakenBack(ann, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Retract(ctx, gone); err != nil {
		t.Fatalf("Retract a sticker: %v", err)
	}
	if got, err := messages.Get(ctx, m.ID()); err != nil || got.Sticker() != "" || !got.IsDeleted() {
		t.Fatalf("taken back = %+v, %v", got, err)
	}
}

// Integration test: needs a database with migrations applied.
func TestChats_Stickers(t *testing.T) {
	svc, ctx := chatService(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	view, err := svc.Create(ctx, ann, "Горы", []uuid.UUID{bob, cat})
	if err != nil {
		t.Fatal(err)
	}
	chatID := view.Chat.ID()

	sent, err := svc.Send(ctx, bob, chatID, service.Draft{Sticker: "smile_wink"})
	if err != nil {
		t.Fatalf("Send a sticker: %v", err)
	}

	if got, _, err := svc.Message(ctx, cat, sent.ID()); err != nil || got.Sticker() != "smile_wink" || got.Text() != "" {
		t.Fatalf("a sticker read back = %+v, %v", got, err)
	}
	if page, err := svc.Chats(ctx, cat, uuid.Nil, 10); err != nil || len(page.Chats) != 1 || page.Chats[0].LastMessage.Sticker() != "smile_wink" {
		t.Fatalf("the chat ends with %+v, %v", page, err)
	}

	if err = svc.Delete(ctx, bob, sent.ID()); err != nil {
		t.Fatalf("take a sticker back: %v", err)
	}
	if got, _, err := svc.Message(ctx, cat, sent.ID()); err != nil || got.Sticker() != "" || !got.IsDeleted() {
		t.Fatalf("taken back = %+v, %v", got, err)
	}
}
