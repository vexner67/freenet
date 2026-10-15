package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
)

func TestMessageService_Reply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	asked := send(t, svc, bob, ann, "идёшь?")[0]
	other := send(t, svc, cat, ann, "другой диалог")[0]

	answer, err := svc.Send(ctx, ann, bob, service.Draft{Sealed: seal("иду"), ReplyTo: asked.ID()})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	q, ok := answer.Quote()
	if !ok || answer.ReplyTo() != asked.ID() || q.ID != asked.ID() || q.SenderID != bob || !q.Found || q.Deleted {
		t.Fatalf("the answer quotes %+v, %v", q, ok)
	}

	for _, tt := range []struct {
		name string
		id   uuid.UUID
	}{
		{"a message of another dialog", other.ID()},
		{"a message that is not there", uuid.New()},
	} {
		if _, err := svc.Send(ctx, ann, bob, service.Draft{Sealed: seal("?"), ReplyTo: tt.id}); !errors.Is(err, message.ErrReplyNotFound) {
			t.Fatalf("%s: %v", tt.name, err)
		}
	}

	if err := svc.Hide(ctx, ann, asked.ID()); err != nil {
		t.Fatalf("Hide: %v", err)
	}
	if _, err := svc.Send(ctx, ann, bob, service.Draft{Sealed: seal("?"), ReplyTo: asked.ID()}); !errors.Is(err, message.ErrReplyNotFound) {
		t.Fatalf("a message hidden from the one answering: %v", err)
	}

	// Bob still has the message: the quote is read with the history.
	page, err := svc.History(ctx, bob, ann, uuid.Nil, 20)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	// The history is oldest first.
	last := page.Messages[len(page.Messages)-1]
	if q, ok := last.Quote(); last.ID() != answer.ID() || !ok || q.ID != asked.ID() || !q.Found {
		t.Fatalf("the history shows %+v", last)
	}

	// What is taken back is quoted as taken back.
	if err := svc.Delete(ctx, bob, asked.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	page, err = svc.History(ctx, bob, ann, uuid.Nil, 20)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if q, _ := page.Messages[len(page.Messages)-1].Quote(); !q.Deleted || !q.Found {
		t.Fatalf("a quote of a message taken back = %+v", q)
	}
}

func TestMessageService_React(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	svc := f.dialogs
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	m := send(t, svc, bob, ann, "привет")[0]

	bobEvents, cancel := f.events.Subscribe(bob)
	defer cancel()

	reacted, err := svc.React(ctx, ann, m.ID(), message.ReactionHeart)
	if err != nil {
		t.Fatalf("React: %v", err)
	}
	if got := reacted.Reactions(); len(got) != 1 || got[0] != (message.Reacted{UserID: ann, Reaction: message.ReactionHeart}) {
		t.Fatalf("reactions = %+v", got)
	}

	if e := <-bobEvents; e.Edited == nil || len(e.Edited.Reactions()) != 1 {
		t.Fatalf("the sender heard %+v", e)
	}

	// A new reaction takes the place of the old one.
	if _, err = svc.React(ctx, ann, m.ID(), message.ReactionFire); err != nil {
		t.Fatalf("React again: %v", err)
	}
	if _, err = svc.React(ctx, bob, m.ID(), message.ReactionLike); err != nil {
		t.Fatalf("React of the sender: %v", err)
	}

	page, err := svc.History(ctx, ann, bob, uuid.Nil, 20)
	if err != nil {
		t.Fatalf("History: %v", err)
	}

	want := message.Reactions{{UserID: ann, Reaction: message.ReactionFire}, {UserID: bob, Reaction: message.ReactionLike}}
	if got := page.Messages[0].Reactions(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("the history shows %+v", got)
	}

	if _, err = svc.React(ctx, cat, m.ID(), message.ReactionHeart); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a stranger reacts: %v", err)
	}

	if reacted, err = svc.React(ctx, ann, m.ID(), ""); err != nil || len(reacted.Reactions()) != 1 {
		t.Fatalf("taking a reaction away: %+v, %v", reacted, err)
	}

	if err = svc.Delete(ctx, bob, m.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err = svc.React(ctx, ann, m.ID(), message.ReactionHeart); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a reaction to a message taken back: %v", err)
	}
}

func TestChatService_ReplyAndReact(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)
	otherChat := chatOf(t, f.svc, dan, ann, bob)

	asked, err := f.svc.Send(ctx, bob, chatID, service.Draft{Text: "кто берёт палатку?"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	elsewhere, err := f.svc.Send(ctx, dan, otherChat, service.Draft{Text: "другая беседа"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	answer, err := f.svc.Send(ctx, ann, chatID, service.Draft{Text: "я", ReplyTo: asked.ID()})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if q, ok := answer.Quote(); !ok || q.Text != "кто берёт палатку?" || q.SenderID != bob {
		t.Fatalf("the answer quotes %+v", q)
	}

	page, err := f.svc.History(ctx, ann, chatID, uuid.Nil, 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}

	// The chat begins with its creation.
	created := page.Messages[0]
	if !created.IsAction() {
		t.Fatalf("the chat begins with %+v", created)
	}

	for _, tt := range []struct {
		name string
		id   uuid.UUID
	}{
		{"a message of another chat", elsewhere.ID()},
		{"an action", created.ID()},
	} {
		if _, err := f.svc.Send(ctx, ann, chatID, service.Draft{Text: "?", ReplyTo: tt.id}); !errors.Is(err, message.ErrReplyNotFound) {
			t.Fatalf("%s: %v", tt.name, err)
		}
	}

	if _, err = f.svc.React(ctx, cat, asked.ID(), message.ReactionLaugh); err != nil {
		t.Fatalf("React: %v", err)
	}
	if _, err = f.svc.React(ctx, cat, created.ID(), message.ReactionLaugh); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a reaction to an action: %v", err)
	}
	if _, err = f.svc.React(ctx, dan, asked.ID(), message.ReactionLaugh); err == nil {
		t.Fatal("a stranger reacts in the chat")
	}

	page, err = f.svc.History(ctx, bob, chatID, uuid.Nil, 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}

	for _, m := range page.Messages {
		if m.ID() == asked.ID() && (len(m.Reactions()) != 1 || m.Reactions()[0].UserID != cat) {
			t.Fatalf("the history shows the reactions %+v", m.Reactions())
		}
	}

	// Who leaves the chat reacts no more.
	if err = f.svc.Leave(ctx, cat, chatID); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if _, err = f.svc.React(ctx, cat, asked.ID(), ""); err == nil {
		t.Fatal("who has left takes a reaction away")
	}

	// An erased page takes its reactions with it.
	if err = f.svc.PurgeUser(ctx, cat); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}

	page, err = f.svc.History(ctx, bob, chatID, uuid.Nil, 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}

	for _, m := range page.Messages {
		if len(m.Reactions()) != 0 {
			t.Fatalf("an erased page left its reaction: %+v", m.Reactions())
		}
	}
}
