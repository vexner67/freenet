package postgres_test

import (
	"context"
	"errors"
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
func TestRepliesAndReactions(t *testing.T) {
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

	messages := postgres.NewMessageRepository(pool)
	reactions := postgres.NewReactionRepository(pool)
	ann, bob := uuid.New(), uuid.New()

	asked, err := message.New(bob, ann, said("идёшь?", nil, uuid.Nil, nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, asked); err != nil {
		t.Fatalf("Save: %v", err)
	}

	answer, err := message.New(ann, bob, said("иду", nil, uuid.Nil, nil).Answering(asked.ID()), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, answer); err != nil {
		t.Fatalf("Save the answer: %v", err)
	}

	t.Run("the answer keeps what it answers", func(t *testing.T) {
		got, err := messages.GetMany(ctx, []uuid.UUID{answer.ID(), asked.ID(), uuid.New()})
		if err != nil || len(got) != 2 {
			t.Fatalf("GetMany = %v, %v", got, err)
		}

		for _, m := range got {
			if m.ID() == answer.ID() && m.ReplyTo() != asked.ID() {
				t.Fatalf("the answer answers %v", m.ReplyTo())
			}
		}
	})

	t.Run("one reaction of each, the oldest first", func(t *testing.T) {
		for _, step := range []struct {
			user uuid.UUID
			r    message.Reaction
		}{{bob, message.ReactionHeart}, {ann, message.ReactionLike}, {bob, message.ReactionFire}} {
			if err := reactions.Set(ctx, asked.ID(), step.user, step.r, time.Now()); err != nil {
				t.Fatalf("Set: %v", err)
			}
		}

		got, err := reactions.Of(ctx, []uuid.UUID{asked.ID(), answer.ID()})
		want := message.Reactions{{UserID: bob, Reaction: message.ReactionFire}, {UserID: ann, Reaction: message.ReactionLike}}
		if err != nil || len(got) != 1 || len(got[asked.ID()]) != 2 || got[asked.ID()][0] != want[0] || got[asked.ID()][1] != want[1] {
			t.Fatalf("Of = %+v, %v", got, err)
		}

		if err := reactions.Set(ctx, uuid.New(), ann, message.ReactionWow, time.Now()); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("a reaction to no message: %v", err)
		}

		if err := reactions.Remove(ctx, asked.ID(), ann); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if got, _ := reactions.Of(ctx, []uuid.UUID{asked.ID()}); len(got[asked.ID()]) != 1 {
			t.Fatalf("after Remove: %+v", got)
		}
	})

	t.Run("a message taken back loses its reactions and its answer", func(t *testing.T) {
		if err := reactions.Set(ctx, answer.ID(), bob, message.ReactionSad, time.Now()); err != nil {
			t.Fatalf("Set: %v", err)
		}

		gone, err := answer.TakenBack(ann, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = messages.Retract(ctx, gone); err != nil {
			t.Fatalf("Retract: %v", err)
		}

		got, err := messages.Get(ctx, answer.ID())
		if err != nil || got.ReplyTo() != uuid.Nil {
			t.Fatalf("the message taken back = %+v, %v", got, err)
		}
		if left, _ := reactions.Of(ctx, []uuid.UUID{answer.ID()}); len(left) != 0 {
			t.Fatalf("its reactions stayed: %+v", left)
		}
	})
}

// Integration test: needs a database with migrations applied.
func TestChatRepliesAndReactions(t *testing.T) {
	svc, ctx := chatServiceOn(t, 0)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	view, err := svc.Create(ctx, ann, "Поход", []uuid.UUID{bob, cat})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	chatID := view.Chat.ID()

	asked, err := svc.Send(ctx, bob, chatID, service.Draft{Text: "кто берёт палатку?"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	answer, err := svc.Send(ctx, ann, chatID, service.Draft{Text: "я", ReplyTo: asked.ID()})
	if err != nil {
		t.Fatalf("Send answering: %v", err)
	}

	if _, err = svc.React(ctx, cat, asked.ID(), message.ReactionLike); err != nil {
		t.Fatalf("React: %v", err)
	}

	page, err := svc.History(ctx, bob, chatID, uuid.Nil, 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}

	for _, m := range page.Messages {
		switch m.ID() {
		case answer.ID():
			if q, ok := m.Quote(); !ok || q.Text != "кто берёт палатку?" {
				t.Fatalf("the answer quotes %+v", q)
			}
		case asked.ID():
			if r := m.Reactions(); len(r) != 1 || r[0].UserID != cat {
				t.Fatalf("the reactions = %+v", r)
			}
		}
	}

	if err = svc.PurgeUser(ctx, cat); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}

	got, err := svc.React(ctx, bob, asked.ID(), message.ReactionHeart)
	if err != nil || len(got.Reactions()) != 1 || got.Reactions()[0].UserID != bob {
		t.Fatalf("after an erased page = %+v, %v", got, err)
	}
}
