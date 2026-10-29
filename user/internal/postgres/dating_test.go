package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestDatingRepository(t *testing.T) {
	repo, pool := newRepo(t)
	dating := postgres.NewDatingRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newUser := func() uuid.UUID {
		t.Helper()

		u, err := user.New(user.Email(unique("dt") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u.ID()
	}
	ann, bob, cid := newUser(), newUser(), newUser()

	if _, err := dating.Card(ctx, ann); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Card before one is made: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	save := func(id uuid.UUID, on bool, at time.Time) {
		t.Helper()

		c, err := user.NewDatingCard(id, user.DatingLove, user.GenderNone, "о себе", on, at)
		if err != nil {
			t.Fatal(err)
		}
		if err = dating.SaveCard(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	save(ann, true, now)
	save(bob, true, now.Add(time.Second))
	save(cid, false, now)
	if err := dating.SaveCard(ctx, user.DatingCard{UserID: uuid.New(), Goal: user.DatingChat, UpdatedAt: now}); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("SaveCard of nobody: %v", err)
	}
	if c, err := dating.Card(ctx, ann); err != nil || c.Goal != user.DatingLove || c.About != "о себе" || !c.On || !c.UpdatedAt.Equal(now) {
		t.Fatalf("Card = %+v, %v", c, err)
	}

	ids := func(cards []user.DatingCard) []uuid.UUID {
		got := make([]uuid.UUID, len(cards))
		for i, c := range cards {
			got[i] = c.UserID
		}

		return got
	}
	cards, err := dating.Candidates(ctx, ann, 1000)
	if err != nil || !slices.Contains(ids(cards), bob) || slices.Contains(ids(cards), ann) || slices.Contains(ids(cards), cid) {
		t.Fatalf("Candidates = %v, %v", ids(cards), err)
	}

	if err = dating.Answer(ctx, ann, bob, true, now); err != nil {
		t.Fatal(err)
	}
	if err = dating.Answer(ctx, ann, bob, false, now); !errors.Is(err, user.ErrDatingTwice) {
		t.Fatalf("a second answer: %v", err)
	}
	if err = dating.Answer(ctx, ann, uuid.New(), true, now); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("an answer to nobody: %v", err)
	}
	if cards, err = dating.Candidates(ctx, ann, 1000); err != nil || slices.Contains(ids(cards), bob) {
		t.Fatalf("Candidates after the answer = %v, %v", ids(cards), err)
	}
	if liked, err := dating.Liked(ctx, ann, bob); err != nil || !liked {
		t.Fatalf("Liked = %v, %v", liked, err)
	}
	if m, err := dating.Matches(ctx, ann); err != nil || len(m) != 0 {
		t.Fatalf("Matches before the answer = %v, %v", m, err)
	}

	later := now.Add(time.Minute)
	if err = dating.Answer(ctx, bob, ann, true, later); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]uuid.UUID{{ann, bob}, {bob, ann}} {
		if m, err := dating.Matches(ctx, pair[0]); err != nil || len(m) != 1 || m[0].UserID != pair[1] || !m[0].At.Equal(later) {
			t.Fatalf("Matches = %+v, %v", m, err)
		}
	}
}
