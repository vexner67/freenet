package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestPollRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	posts := postgres.NewPostRepository(pool)
	polls := postgres.NewPollRepository(pool)

	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()

	title, err := post.NewPollTitle("Кошки или собаки?")
	if err != nil {
		t.Fatal(err)
	}
	options, err := post.NewPollOptions([]string{"кошки", "собаки", "рыбки"})
	if err != nil {
		t.Fatal(err)
	}

	// A post of a poll alone passes the content check of the table.
	p, err := post.New(ann, ann, post.NewPolled("", nil), now)
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Save(ctx, p); err != nil {
		t.Fatalf("save a post of a poll alone: %v", err)
	}

	asked, err := post.NewPoll(p.ID(), post.PollDraft{Title: title, Options: options, Multiple: true, Days: 3}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = polls.Save(ctx, asked); err != nil {
		t.Fatalf("Save: %v", err)
	}

	stored, err := posts.Get(ctx, p.ID())
	if err != nil || !stored.Content().Polled() {
		t.Fatalf("the stored post does not ask a poll: %v", err)
	}

	got, err := polls.Get(ctx, p.ID())
	if err != nil || !reflect.DeepEqual(got.Stored(), asked.Stored()) {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, asked.Stored())
	}

	if _, err = polls.Get(ctx, uuid.New()); !errors.Is(err, post.ErrPollNotFound) {
		t.Fatalf("Get of no poll: %v", err)
	}

	many, err := polls.GetMany(ctx, []uuid.UUID{uuid.New(), p.ID()})
	if err != nil || len(many) != 1 || many[0].PostID() != p.ID() {
		t.Fatalf("GetMany = %v, %v", many, err)
	}

	err = pgpool.NewTxManager(pool).WithinTx(ctx, func(ctx context.Context) error {
		locked, err := polls.Lock(ctx, p.ID())
		if err != nil {
			return err
		}
		if locked.PostID() != p.ID() {
			t.Errorf("Lock = %+v", locked)
		}

		return polls.Vote(ctx, p.ID(), bob, post.PollChoice{0, 2}, now)
	})
	if err != nil {
		t.Fatalf("vote in a transaction: %v", err)
	}

	if err = polls.Vote(ctx, p.ID(), bob, post.PollChoice{0}, now); !errors.Is(err, post.ErrPollVoted) {
		t.Fatalf("a second vote: %v", err)
	}
	if err = polls.Vote(ctx, p.ID(), cat, post.PollChoice{0}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	tally := func() post.PollTally {
		t.Helper()

		poll, err := polls.Get(ctx, p.ID())
		if err != nil {
			t.Fatal(err)
		}

		return poll.Stored().Tally
	}

	if got := tally(); !reflect.DeepEqual(got, post.PollTally{Votes: []int{2, 0, 1}, Voters: 2}) {
		t.Fatalf("tally = %+v", got)
	}

	chosen, err := polls.Chosen(ctx, bob, []uuid.UUID{p.ID(), uuid.New()})
	if err != nil || len(chosen) != 1 || !reflect.DeepEqual(chosen[p.ID()], post.PollChoice{0, 2}) {
		t.Fatalf("Chosen = %v, %v", chosen, err)
	}

	// Who voted for an option, newest first, a page at a time.
	first, err := polls.Voters(ctx, p.ID(), 0, nil, 1)
	if err != nil || len(first) != 1 || first[0].UserID != cat {
		t.Fatalf("Voters = %v, %v", first, err)
	}

	rest, err := polls.Voters(ctx, p.ID(), 0, &service.LikerCursor{LikedAt: first[0].LikedAt, UserID: first[0].UserID}, 5)
	if err != nil || len(rest) != 1 || rest[0].UserID != bob {
		t.Fatalf("the second page of voters = %v, %v", rest, err)
	}

	if err = polls.Retract(ctx, p.ID(), cat); err != nil {
		t.Fatal(err)
	}
	// Nothing to take back: nothing changes.
	if err = polls.Retract(ctx, p.ID(), cat); err != nil {
		t.Fatal(err)
	}
	if got := tally(); !reflect.DeepEqual(got, post.PollTally{Votes: []int{1, 0, 1}, Voters: 1}) {
		t.Fatalf("tally after a retracted vote = %+v", got)
	}

	if err = polls.DeleteByUser(ctx, bob); err != nil {
		t.Fatal(err)
	}
	if got := tally(); !reflect.DeepEqual(got, post.PollTally{Votes: []int{0, 0, 0}, Voters: 0}) {
		t.Fatalf("tally after the voter is purged = %+v", got)
	}

	// The poll goes with its post.
	if err = polls.Vote(ctx, p.ID(), bob, post.PollChoice{1}, now); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "DELETE FROM posts WHERE id = $1", p.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = polls.Get(ctx, p.ID()); !errors.Is(err, post.ErrPollNotFound) {
		t.Fatalf("the poll of a deleted post: %v", err)
	}

	var votes int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM poll_votes WHERE post_id = $1", p.ID()).Scan(&votes); err != nil || votes != 0 {
		t.Fatalf("votes of a deleted post = %d, %v", votes, err)
	}
}
