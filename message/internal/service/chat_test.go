package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/inmem"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/pkg/pubsub"
)

type chatFixture struct {
	svc    *service.ChatService
	events *pubsub.Hub[uuid.UUID, service.Event]
	photos *fakePhotos
}

func newChats() chatFixture {
	store := inmem.NewStore()
	f := chatFixture{events: pubsub.New[uuid.UUID, service.Event](64), photos: &fakePhotos{}}
	f.svc = service.NewChatService(service.ChatDeps{
		Chats: store.Chats(), Members: store.ChatMembers(), Messages: store.ChatMessages(), Events: f.events, Tx: fakeTx{}, Photos: f.photos,
	})

	return f
}

// chatOf starts a chat of owner with the others and returns its ID.
func chatOf(t *testing.T, svc *service.ChatService, owner uuid.UUID, others ...uuid.UUID) uuid.UUID {
	t.Helper()

	view, err := svc.Create(context.Background(), owner, "Поход", others)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	return view.Chat.ID()
}

// actions are what the messages of a chat tell of, "" for words.
func actions(messages []*message.ChatMessage) []message.Action {
	out := make([]message.Action, len(messages))
	for i, m := range messages {
		out[i] = m.Action()
	}

	return out
}

func TestChatService_Create(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat, stranger := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	if _, err := f.svc.Create(ctx, ann, "Вдвоём", []uuid.UUID{bob, ann}); !errors.Is(err, message.ErrTooFewMembers) {
		t.Fatalf("a chat of two: %v", err)
	}

	bobEvents, cancel := f.events.Subscribe(bob)
	defer cancel()

	view, err := f.svc.Create(ctx, ann, "Поход", []uuid.UUID{bob, cat, bob})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.Chat.OwnerID() != ann || view.Chat.Title() != "Поход" || view.Members != 3 || !view.Me.IsActive() || view.Me.HasUnread() {
		t.Fatalf("the creator's view = %+v", view)
	}
	if view.LastMessage == nil || view.LastMessage.Action() != message.ActionCreate || view.LastMessage.Text() != "Поход" {
		t.Fatalf("the chat begins with %+v", view.LastMessage)
	}

	if e := <-bobEvents; e.Chat == nil || e.Chat.Message == nil || e.Chat.Message.ID() != view.LastMessage.ID() {
		t.Fatalf("an invited member heard %+v", e)
	}

	details, err := f.svc.Get(ctx, bob, view.Chat.ID())
	if err != nil || len(details.Members) != 3 || details.Members[0].UserID != ann || !details.View.Me.HasUnread() {
		t.Fatalf("Get as an invited member = %+v, %v", details, err)
	}

	if _, err = f.svc.Get(ctx, stranger, view.Chat.ID()); !errors.Is(err, message.ErrChatNotFound) {
		t.Fatalf("a stranger opens the chat: %v", err)
	}
	if _, err = f.svc.History(ctx, stranger, view.Chat.ID(), uuid.Nil, 20); !errors.Is(err, message.ErrChatNotFound) {
		t.Fatalf("a stranger reads the chat: %v", err)
	}
	if _, err = f.svc.Send(ctx, stranger, view.Chat.ID(), "я тут", nil); !errors.Is(err, message.ErrChatNotFound) {
		t.Fatalf("a stranger writes into the chat: %v", err)
	}

	for _, id := range []uuid.UUID{ann, bob, cat} {
		page, err := f.svc.Chats(ctx, id, uuid.Nil, 20)
		if err != nil || len(page.Chats) != 1 || page.Chats[0].Chat.ID() != view.Chat.ID() {
			t.Fatalf("chats of a member = %+v, %v", page, err)
		}
	}

	if page, err := f.svc.Chats(ctx, stranger, uuid.Nil, 20); err != nil || len(page.Chats) != 0 {
		t.Fatalf("chats of a stranger = %+v, %v", page, err)
	}
}

func TestChatService_Messages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)

	photo := uuid.New()

	first, err := f.svc.Send(ctx, bob, chatID, "привет", message.Photos{photo})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(f.photos.published) != 1 || f.photos.published[0].owner != bob || f.photos.outsideTx != 0 {
		t.Fatalf("media heard %+v, outside the transaction %d", f.photos.published, f.photos.outsideTx)
	}

	second, err := f.svc.Send(ctx, cat, chatID, "и вам", nil)
	if err != nil {
		t.Fatal(err)
	}

	if n, err := f.svc.CountUnread(ctx, ann); err != nil || n != 1 {
		t.Fatalf("unread chats of ann = %d, %v", n, err)
	}

	page, err := f.svc.History(ctx, ann, chatID, uuid.Nil, 2)
	if err != nil || len(page.Messages) != 2 || page.Messages[0].ID() != first.ID() || page.Messages[1].ID() != second.ID() || page.NextCursor != first.ID() {
		t.Fatalf("the newest page = %+v, %v", page, err)
	}

	older, err := f.svc.History(ctx, ann, chatID, page.NextCursor, 2)
	if err != nil || len(older.Messages) != 1 || older.Messages[0].Action() != message.ActionCreate || older.NextCursor != uuid.Nil {
		t.Fatalf("the older page = %+v, %v", older, err)
	}

	newer, err := f.svc.Newer(ctx, ann, chatID, first.ID(), 20)
	if err != nil || len(newer.Messages) != 1 || newer.Messages[0].ID() != second.ID() {
		t.Fatalf("Newer = %+v, %v", newer, err)
	}

	t.Run("reading", func(t *testing.T) {
		annEvents, cancel := f.events.Subscribe(ann)
		defer cancel()

		if err := f.svc.MarkRead(ctx, ann, chatID, uuid.Must(uuid.NewV7())); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("read up to a message made up: %v", err)
		}

		if err := f.svc.MarkRead(ctx, ann, chatID, second.ID()); err != nil {
			t.Fatalf("MarkRead: %v", err)
		}
		if e := <-annEvents; e.Chat == nil || e.Chat.Read == nil || e.Chat.Read.UpTo != second.ID() || e.Chat.Read.ChatID != chatID {
			t.Fatalf("the reader's other pages heard %+v", e)
		}

		// The mark never moves back.
		if err := f.svc.MarkRead(ctx, ann, chatID, first.ID()); err != nil {
			t.Fatal(err)
		}
		if n, err := f.svc.CountUnread(ctx, ann); err != nil || n != 0 {
			t.Fatalf("unread chats of ann after reading = %d, %v", n, err)
		}

		// Bob wrote first and has not read what cat answered.
		if n, err := f.svc.CountUnread(ctx, bob); err != nil || n != 1 {
			t.Fatalf("unread chats of bob = %d, %v", n, err)
		}
	})

	t.Run("edit and take back", func(t *testing.T) {
		catEvents, cancel := f.events.Subscribe(cat)
		defer cancel()

		if _, err := f.svc.Edit(ctx, ann, first.ID(), "не моё"); !errors.Is(err, message.ErrCannotEdit) {
			t.Fatalf("someone else edits: %v", err)
		}

		edited, err := f.svc.Edit(ctx, bob, first.ID(), "привет всем")
		if err != nil || edited.Text() != "привет всем" || len(edited.Photos()) != 1 {
			t.Fatalf("Edit = %+v, %v", edited, err)
		}
		if e := <-catEvents; e.Chat == nil || e.Chat.Edited == nil || e.Chat.Edited.Text() != "привет всем" {
			t.Fatalf("the chat heard %+v", e)
		}

		if err = f.svc.Delete(ctx, ann, first.ID()); !errors.Is(err, message.ErrCannotDelete) {
			t.Fatalf("someone else takes back: %v", err)
		}
		if err = f.svc.Delete(ctx, bob, first.ID()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if e := <-catEvents; e.Chat == nil || e.Chat.Edited == nil || !e.Chat.Edited.IsDeleted() {
			t.Fatalf("the chat heard %+v", e)
		}
		if len(f.photos.deleted) != 1 || f.photos.deleted[0].owner != bob || !slices.Equal(f.photos.deleted[0].photos, []uuid.UUID{photo}) || f.photos.outsideTx != 0 {
			t.Fatalf("media heard %+v", f.photos.deleted)
		}

		got, _, err := f.svc.Message(ctx, cat, first.ID())
		if err != nil || !got.IsDeleted() || got.Text() != "" {
			t.Fatalf("the message taken back = %+v, %v", got, err)
		}
		if err = f.svc.Delete(ctx, bob, first.ID()); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("taken back twice: %v", err)
		}
	})
}

func TestChatService_Members(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)

	history := func(userID uuid.UUID) []message.Action {
		t.Helper()

		page, err := f.svc.History(ctx, userID, chatID, uuid.Nil, 50)
		if err != nil {
			t.Fatalf("History: %v", err)
		}

		return actions(page.Messages)
	}

	// Anyone in the chat adds people; who is in it already stays.
	if err := f.svc.Invite(ctx, bob, chatID, []uuid.UUID{dan, cat, bob}); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if got := history(dan); !slices.Equal(got, []message.Action{message.ActionCreate, message.ActionInvite}) {
		t.Fatalf("a new member reads %v", got)
	}

	if err := f.svc.Rename(ctx, dan, chatID, "Горы"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if view, err := f.svc.Get(ctx, ann, chatID); err != nil || view.View.Chat.Title() != "Горы" || view.View.Members != 4 ||
		view.View.LastMessage.Action() != message.ActionTitle || view.View.LastMessage.Text() != "Горы" {
		t.Fatalf("the chat renamed = %+v, %v", view, err)
	}

	// The same title again tells the chat nothing.
	if err := f.svc.Rename(ctx, dan, chatID, "Горы"); err != nil || len(history(ann)) != 3 {
		t.Fatalf("renamed to the same: %v, %v", err, history(ann))
	}

	t.Run("only the creator removes", func(t *testing.T) {
		if err := f.svc.Kick(ctx, bob, chatID, cat); !errors.Is(err, message.ErrCannotKick) {
			t.Fatalf("a member removes: %v", err)
		}
		if err := f.svc.Kick(ctx, ann, chatID, ann); !errors.Is(err, message.ErrKickYourself) {
			t.Fatalf("the creator removes themselves: %v", err)
		}
		if err := f.svc.Kick(ctx, ann, chatID, uuid.New()); !errors.Is(err, message.ErrMemberNotFound) {
			t.Fatalf("a stranger is removed: %v", err)
		}

		catEvents, cancel := f.events.Subscribe(cat)
		defer cancel()

		if err := f.svc.Kick(ctx, ann, chatID, cat); err != nil {
			t.Fatalf("Kick: %v", err)
		}
		if e := <-catEvents; e.Chat == nil || e.Chat.Message == nil || e.Chat.Message.Action() != message.ActionKick || e.Chat.Message.TargetID() != cat {
			t.Fatalf("who was removed heard %+v", e)
		}

		if _, err := f.svc.Send(ctx, ann, chatID, "без кати", nil); err != nil {
			t.Fatal(err)
		}

		// Cat keeps the chat as it was when she went, and does nothing in it.
		got := history(cat)
		if got[len(got)-1] != message.ActionKick {
			t.Fatalf("who was removed reads %v", got)
		}

		details, err := f.svc.Get(ctx, cat, chatID)
		if err != nil || details.View.Me.State != message.MemberKicked || len(details.Members) != 0 || details.View.Members != 3 ||
			details.View.LastMessage.Action() != message.ActionKick {
			t.Fatalf("the chat of who was removed = %+v, %v", details, err)
		}

		for name, err := range map[string]error{
			"write":  second(f.svc.Send(ctx, cat, chatID, "я тут", nil)),
			"rename": f.svc.Rename(ctx, cat, chatID, "Моё"),
			"invite": f.svc.Invite(ctx, cat, chatID, []uuid.UUID{uuid.New()}),
			"leave":  f.svc.Leave(ctx, cat, chatID),
		} {
			if !errors.Is(err, message.ErrNotInChat) {
				t.Errorf("who was removed may %s: %v", name, err)
			}
		}

		if err = f.svc.Return(ctx, cat, chatID); !errors.Is(err, message.ErrKicked) {
			t.Fatalf("who was removed comes back on their own: %v", err)
		}

		// Someone adds her again: she reads everything.
		if err = f.svc.Invite(ctx, bob, chatID, []uuid.UUID{cat}); err != nil {
			t.Fatalf("Invite again: %v", err)
		}
		if got = history(cat); got[len(got)-1] != message.ActionInvite || !slices.Contains(got, message.ActionNone) {
			t.Fatalf("who was added again reads %v", got)
		}
	})

	t.Run("leaving and coming back", func(t *testing.T) {
		if err := f.svc.Leave(ctx, dan, chatID); err != nil {
			t.Fatalf("Leave: %v", err)
		}
		if err := f.svc.Leave(ctx, dan, chatID); !errors.Is(err, message.ErrNotInChat) {
			t.Fatalf("left twice: %v", err)
		}
		if err := f.svc.Invite(ctx, ann, chatID, []uuid.UUID{dan}); !errors.Is(err, message.ErrMemberLeft) {
			t.Fatalf("who left is brought back by others: %v", err)
		}

		sent, err := f.svc.Send(ctx, ann, chatID, "без дани", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = f.svc.Message(ctx, dan, sent.ID()); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("who left reads what came after: %v", err)
		}
		if err = f.svc.MarkRead(ctx, dan, chatID, sent.ID()); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("who left reads up to what came after: %v", err)
		}

		if got := history(dan); got[len(got)-1] != message.ActionLeave {
			t.Fatalf("who left reads %v", got)
		}

		if err = f.svc.Return(ctx, dan, chatID); err != nil {
			t.Fatalf("Return: %v", err)
		}
		// Coming back twice changes nothing.
		if err = f.svc.Return(ctx, dan, chatID); err != nil {
			t.Fatalf("Return twice: %v", err)
		}

		got := history(dan)
		if got[len(got)-1] != message.ActionReturn || got[len(got)-2] != message.ActionNone {
			t.Fatalf("who came back reads %v", got)
		}
	})
}

// second is the error of a call that returns something else first.
func second[T any](_ T, err error) error {
	return err
}

func TestChatService_Full(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann := uuid.New()

	others := make([]uuid.UUID, message.MaxChatMembers-2)
	for i := range others {
		others[i] = uuid.New()
	}

	chatID := chatOf(t, f.svc, ann, others...)

	if err := f.svc.Invite(ctx, ann, chatID, []uuid.UUID{uuid.New(), uuid.New()}); !errors.Is(err, message.ErrChatFull) {
		t.Fatalf("two join a chat with one place: %v", err)
	}

	// Who left gives up their place and waits for another.
	if err := f.svc.Leave(ctx, others[0], chatID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Invite(ctx, ann, chatID, []uuid.UUID{uuid.New(), uuid.New()}); err != nil {
		t.Fatalf("two join a chat with two places: %v", err)
	}
	if err := f.svc.Return(ctx, others[0], chatID); !errors.Is(err, message.ErrChatFull) {
		t.Fatalf("coming back into a full chat: %v", err)
	}
}

func TestChatService_PurgeUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)

	kept, err := f.svc.Send(ctx, bob, chatID, "останется", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.Send(ctx, ann, chatID, "уйдёт", nil); err != nil {
		t.Fatal(err)
	}

	if err = f.svc.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}
	// Idempotent.
	if err = f.svc.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("PurgeUser again: %v", err)
	}

	if _, err = f.svc.Get(ctx, ann, chatID); !errors.Is(err, message.ErrChatNotFound) {
		t.Fatalf("the chat of who is gone: %v", err)
	}

	details, err := f.svc.Get(ctx, cat, chatID)
	if err != nil || details.View.Members != 2 || details.View.Chat.OwnerID() != bob {
		t.Fatalf("the chat after its creator went = %+v, %v", details, err)
	}
	// What the creator wrote went with them: the chat is back to what is left.
	if details.View.LastMessage == nil || details.View.LastMessage.ID() != kept.ID() {
		t.Fatalf("the last message = %+v, want what bob wrote", details.View.LastMessage)
	}

	page, err := f.svc.History(ctx, cat, chatID, uuid.Nil, 20)
	if err != nil || len(page.Messages) != 1 {
		t.Fatalf("the chat keeps %d messages, %v; want 1", len(page.Messages), err)
	}

	if _, err = f.svc.Send(ctx, bob, chatID, "дальше", nil); err != nil {
		t.Fatalf("the chat goes on: %v", err)
	}
}
