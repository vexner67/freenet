package message_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
)

func TestNewReaction(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"heart", "like", "laugh", "wow", "sad", "fire"} {
		if r, err := message.NewReaction(s); err != nil || r.String() != s {
			t.Fatalf("NewReaction(%q) = %q, %v", s, r, err)
		}
	}

	for _, s := range []string{"", "Heart", "❤️", "angry"} {
		if _, err := message.NewReaction(s); !errors.Is(err, message.ErrInvalidReaction) {
			t.Fatalf("NewReaction(%q): %v", s, err)
		}
	}
}

func TestMessage_AnswerAndReact(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	m := message.Restore(message.Stored{ID: uuid.New(), SenderID: ann, RecipientID: bob, Content: message.RestoreContent("", message.Photos{uuid.New()}, uuid.Nil, nil), CreatedAt: now})

	if err := m.CheckAnswer(bob); err != nil {
		t.Fatalf("the recipient answers: %v", err)
	}
	if err := m.CheckAnswer(cat); !errors.Is(err, message.ErrReplyNotFound) {
		t.Fatalf("a stranger answers: %v", err)
	}
	if err := m.CheckReact(cat); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a stranger reacts: %v", err)
	}

	q := message.QuoteOf(m)
	if q.ID != m.ID() || q.SenderID != ann || q.Photos != 1 || !q.Found || q.Deleted {
		t.Fatalf("QuoteOf = %+v", q)
	}

	// Showing a quote and reactions makes another message: those read
	// before keep theirs.
	reacted := m.Quoting(message.Lost(uuid.New())).Reacting(message.Reactions{{UserID: bob, Reaction: message.ReactionLike}})
	if _, ok := m.Quote(); ok || m.Reactions() != nil || len(reacted.Reactions()) != 1 {
		t.Fatalf("the message read before changed: %+v", m)
	}

	gone, err := reacted.TakenBack(ann, now)
	if err != nil {
		t.Fatalf("TakenBack: %v", err)
	}
	if _, ok := gone.Quote(); ok || gone.Reactions() != nil {
		t.Fatalf("a message taken back shows %+v", gone)
	}
	if err := gone.CheckAnswer(bob); !errors.Is(err, message.ErrReplyNotFound) {
		t.Fatalf("an answer to a message taken back: %v", err)
	}
	if err := gone.CheckReact(bob); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a reaction to a message taken back: %v", err)
	}
}
