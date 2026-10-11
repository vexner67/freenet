package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/mediaclient"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/postgres"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/pubsub"
)

// chatService is the chats on a real database, as app.New wires them.
func chatService(t *testing.T) (*service.ChatService, context.Context) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	return service.NewChatService(service.ChatDeps{
		Chats:    postgres.NewChatRepository(pool),
		Members:  postgres.NewChatMemberRepository(pool),
		Messages: postgres.NewChatMessageRepository(pool),
		Events:   pubsub.New[uuid.UUID, service.Event](64),
		Tx:       pgpool.NewTxManager(pool),
		Photos:   mediaclient.Nop{},
	}), ctx
}

// Integration test: needs a database with migrations applied.
func TestChats(t *testing.T) {
	svc, ctx := chatService(t)
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	view, err := svc.Create(ctx, ann, "Поход", []uuid.UUID{bob, cat})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	chatID := view.Chat.ID()

	if view.Members != 3 || view.Chat.Title() != "Поход" || view.Chat.OwnerID() != ann || view.Me.HasUnread() ||
		view.LastMessage == nil || view.LastMessage.Action() != message.ActionCreate || view.LastMessage.Text() != "Поход" {
		t.Fatalf("the creator's view = %+v, last %+v", view, view.LastMessage)
	}

	photos := message.Photos{uuid.New(), uuid.New()}

	first, err := svc.Send(ctx, bob, chatID, "привет", photos)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	got, me, err := svc.Message(ctx, cat, first.ID())
	if err != nil || got.Text() != "привет" || len(got.Photos()) != 2 || got.Photos()[0] != photos[0] || !got.CreatedAt().Equal(first.CreatedAt()) ||
		got.ChatID() != chatID || got.SenderID() != bob || got.IsAction() || !me.HasUnread() {
		t.Fatalf("the message read back = %+v, %+v, %v", got, me, err)
	}

	t.Run("lists and counters", func(t *testing.T) {
		for _, tt := range []struct {
			user   uuid.UUID
			unread int
		}{
			{ann, 1}, {bob, 1}, {cat, 1}, {dan, 0},
		} {
			if n, err := svc.CountUnread(ctx, tt.user); err != nil || n != tt.unread {
				t.Fatalf("CountUnread(%v) = %d, %v; want %d", tt.user, n, err, tt.unread)
			}
		}

		page, err := svc.Chats(ctx, ann, uuid.Nil, 20)
		if err != nil || len(page.Chats) != 1 || page.Chats[0].LastMessage.ID() != first.ID() || !page.Chats[0].Me.HasUnread() || page.Chats[0].Members != 3 {
			t.Fatalf("Chats = %+v, %v", page, err)
		}

		// Nothing is below the last message of the only chat.
		if page, err = svc.Chats(ctx, ann, first.ID(), 20); err != nil || len(page.Chats) != 0 {
			t.Fatalf("Chats below the newest = %+v, %v", page, err)
		}

		if err = svc.MarkRead(ctx, ann, chatID, first.ID()); err != nil {
			t.Fatalf("MarkRead: %v", err)
		}
		if n, err := svc.CountUnread(ctx, ann); err != nil || n != 0 {
			t.Fatalf("CountUnread after reading = %d, %v", n, err)
		}

		// The mark never moves back.
		if err = svc.MarkRead(ctx, ann, chatID, view.LastMessage.ID()); err != nil {
			t.Fatal(err)
		}
		if n, _ := svc.CountUnread(ctx, ann); n != 0 {
			t.Fatalf("the mark moved back: %d unread", n)
		}
	})

	t.Run("history", func(t *testing.T) {
		second, err := svc.Send(ctx, cat, chatID, "и вам", nil)
		if err != nil {
			t.Fatal(err)
		}

		page, err := svc.History(ctx, ann, chatID, uuid.Nil, 2)
		if err != nil || len(page.Messages) != 2 || page.Messages[0].ID() != first.ID() || page.Messages[1].ID() != second.ID() || page.NextCursor != first.ID() {
			t.Fatalf("History = %+v, %v", page, err)
		}

		older, err := svc.History(ctx, ann, chatID, page.NextCursor, 2)
		if err != nil || len(older.Messages) != 1 || older.Messages[0].Action() != message.ActionCreate || older.NextCursor != uuid.Nil {
			t.Fatalf("older = %+v, %v", older, err)
		}

		newer, err := svc.Newer(ctx, ann, chatID, first.ID(), 20)
		if err != nil || len(newer.Messages) != 1 || newer.Messages[0].ID() != second.ID() {
			t.Fatalf("Newer = %+v, %v", newer, err)
		}
	})

	t.Run("edit and take back", func(t *testing.T) {
		edited, err := svc.Edit(ctx, bob, first.ID(), "привет всем")
		if err != nil {
			t.Fatalf("Edit: %v", err)
		}

		got, _, err := svc.Message(ctx, ann, first.ID())
		if err != nil || got.Text() != "привет всем" || !got.EditedAt().Equal(edited.EditedAt()) || len(got.Photos()) != 2 {
			t.Fatalf("the edited message = %+v, %v", got, err)
		}

		if err = svc.Delete(ctx, bob, first.ID()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if got, _, err = svc.Message(ctx, ann, first.ID()); err != nil || !got.IsDeleted() || got.Text() != "" || len(got.Photos()) != 0 || !got.EditedAt().IsZero() {
			t.Fatalf("the message taken back = %+v, %v", got, err)
		}

		if err = svc.Delete(ctx, bob, first.ID()); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("taken back twice: %v", err)
		}
		if _, err = svc.Edit(ctx, bob, first.ID(), "снова"); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("a message taken back is edited: %v", err)
		}
	})

	t.Run("people come and go", func(t *testing.T) {
		if err := svc.Invite(ctx, bob, chatID, []uuid.UUID{dan, cat}); err != nil {
			t.Fatalf("Invite: %v", err)
		}
		if err := svc.Rename(ctx, dan, chatID, "Горы"); err != nil {
			t.Fatalf("Rename: %v", err)
		}
		if err := svc.Kick(ctx, ann, chatID, cat); err != nil {
			t.Fatalf("Kick: %v", err)
		}

		after, err := svc.Send(ctx, ann, chatID, "без кати", nil)
		if err != nil {
			t.Fatal(err)
		}

		details, err := svc.Get(ctx, cat, chatID)
		if err != nil || details.View.Me.State != message.MemberKicked || len(details.Members) != 0 || details.View.Members != 3 ||
			details.View.Chat.Title() != "Горы" || details.View.LastMessage.Action() != message.ActionKick || details.View.LastMessage.TargetID() != cat {
			t.Fatalf("the chat of who was removed = %+v, last %+v, %v", details, details.View.LastMessage, err)
		}

		if _, _, err = svc.Message(ctx, cat, after.ID()); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("who was removed reads what came after: %v", err)
		}
		if page, err := svc.History(ctx, cat, chatID, uuid.Nil, 50); err != nil || page.Messages[len(page.Messages)-1].Action() != message.ActionKick {
			t.Fatalf("the history of who was removed ends with %+v, %v", page.Messages, err)
		}
		if _, err = svc.Send(ctx, cat, chatID, "я тут", nil); !errors.Is(err, message.ErrNotInChat) {
			t.Fatalf("who was removed writes: %v", err)
		}

		if err = svc.Leave(ctx, dan, chatID); err != nil {
			t.Fatalf("Leave: %v", err)
		}
		if err = svc.Invite(ctx, ann, chatID, []uuid.UUID{dan}); !errors.Is(err, message.ErrMemberLeft) {
			t.Fatalf("who left is brought back by others: %v", err)
		}
		if err = svc.Return(ctx, dan, chatID); err != nil {
			t.Fatalf("Return: %v", err)
		}

		details, err = svc.Get(ctx, dan, chatID)
		if err != nil || !details.View.Me.IsActive() || len(details.Members) != 3 || details.Members[0].UserID != ann ||
			details.View.LastMessage.Action() != message.ActionReturn {
			t.Fatalf("the chat of who came back = %+v, %v", details, err)
		}
	})

	t.Run("a stranger", func(t *testing.T) {
		stranger := uuid.New()

		if _, err := svc.Get(ctx, stranger, chatID); !errors.Is(err, message.ErrChatNotFound) {
			t.Fatalf("Get: %v", err)
		}
		if _, err := svc.Get(ctx, ann, uuid.New()); !errors.Is(err, message.ErrChatNotFound) {
			t.Fatalf("Get of no chat: %v", err)
		}
		if _, err := svc.Send(ctx, ann, uuid.New(), "куда", nil); !errors.Is(err, message.ErrChatNotFound) {
			t.Fatalf("Send to no chat: %v", err)
		}
		if _, _, err := svc.Message(ctx, stranger, first.ID()); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("Message: %v", err)
		}
	})
}

// Messages sent at once into one chat reach everyone in it: no row of a
// member is left behind the newest message.
func TestChats_ConcurrentSend(t *testing.T) {
	svc, ctx := chatService(t)
	people := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}

	view, err := svc.Create(ctx, people[0], "", people[1:])
	if err != nil {
		t.Fatal(err)
	}

	const each = 5

	var wg sync.WaitGroup
	for _, sender := range people {
		for range each {
			wg.Add(1)

			go func() {
				defer wg.Done()

				if _, err := svc.Send(ctx, sender, view.Chat.ID(), "вместе", nil); err != nil {
					t.Errorf("Send: %v", err)
				}
			}()
		}
	}

	wg.Wait()

	page, err := svc.History(ctx, people[0], view.Chat.ID(), uuid.Nil, 50)
	if err != nil || len(page.Messages) != len(people)*each+1 {
		t.Fatalf("the chat has %d messages, %v; want %d", len(page.Messages), err, len(people)*each+1)
	}

	newest := page.Messages[len(page.Messages)-1].ID()

	for _, id := range people {
		got, err := svc.Get(ctx, id, view.Chat.ID())
		if err != nil || got.View.LastMessage.ID() != newest {
			t.Fatalf("a member is at %v, %v; want the newest %v", got.View.LastMessage.ID(), err, newest)
		}
	}
}

// Adding people at once never fills a chat past its room.
func TestChats_ConcurrentInvite(t *testing.T) {
	svc, ctx := chatService(t)
	owner := uuid.New()

	others := make([]uuid.UUID, message.MaxChatMembers-4)
	for i := range others {
		others[i] = uuid.New()
	}

	view, err := svc.Create(ctx, owner, "Почти полная", others)
	if err != nil {
		t.Fatal(err)
	}

	// Three places are left, and six pairs want them.
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			err := svc.Invite(ctx, owner, view.Chat.ID(), []uuid.UUID{uuid.New(), uuid.New()})
			if err != nil && !errors.Is(err, message.ErrChatFull) {
				t.Errorf("Invite: %v", err)
			}
		}()
	}

	wg.Wait()

	got, err := svc.Get(ctx, owner, view.Chat.ID())
	if err != nil || got.View.Members != message.MaxChatMembers-1 || len(got.Members) != got.View.Members {
		t.Fatalf("the chat has %d people, %v; want %d", got.View.Members, err, message.MaxChatMembers-1)
	}
}

func TestChats_PurgeUser(t *testing.T) {
	svc, ctx := chatService(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	view, err := svc.Create(ctx, ann, "Поход", []uuid.UUID{bob, cat})
	if err != nil {
		t.Fatal(err)
	}
	chatID := view.Chat.ID()

	// A chat of others, which nothing happens to.
	other, err := svc.Create(ctx, bob, "Без Анны", []uuid.UUID{cat, uuid.New()})
	if err != nil {
		t.Fatal(err)
	}

	kept, err := svc.Send(ctx, bob, chatID, "останется", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.MarkRead(ctx, cat, chatID, kept.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, ann, chatID, "уйдёт", nil); err != nil {
		t.Fatal(err)
	}
	if err = svc.Kick(ctx, ann, chatID, cat); err != nil {
		t.Fatal(err)
	}

	if err = svc.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}
	// Idempotent.
	if err = svc.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("PurgeUser again: %v", err)
	}

	if _, err = svc.Get(ctx, ann, chatID); !errors.Is(err, message.ErrChatNotFound) {
		t.Fatalf("the chat of who is gone: %v", err)
	}

	// Bob, who has been in it longest, has the chat now; what ann wrote
	// and did went with her.
	details, err := svc.Get(ctx, bob, chatID)
	if err != nil || details.View.Chat.OwnerID() != bob || details.View.Members != 1 || details.View.LastMessage == nil ||
		details.View.LastMessage.ID() != kept.ID() || details.View.Me.HasUnread() {
		t.Fatalf("the chat after its creator went = %+v, last %+v, %v", details, details.View.LastMessage, err)
	}

	// Cat, removed by ann, keeps what is left of what she had and has
	// read it all.
	gone, err := svc.Get(ctx, cat, chatID)
	if err != nil || gone.View.Me.State != message.MemberKicked || gone.View.LastMessage == nil || gone.View.LastMessage.ID() != kept.ID() || gone.View.Me.HasUnread() {
		t.Fatalf("the chat of who was removed = %+v, last %+v, %v", gone, gone.View.LastMessage, err)
	}

	page, err := svc.History(ctx, bob, chatID, uuid.Nil, 20)
	if err != nil || len(page.Messages) != 1 {
		t.Fatalf("the chat keeps %d messages, %v; want 1", len(page.Messages), err)
	}

	if untouched, err := svc.Get(ctx, bob, other.Chat.ID()); err != nil || untouched.View.Members != 3 || untouched.View.Chat.OwnerID() != bob {
		t.Fatalf("a chat of others = %+v, %v", untouched, err)
	}

	if _, err = svc.Send(ctx, bob, chatID, "дальше", nil); err != nil {
		t.Fatalf("the chat goes on: %v", err)
	}
}
