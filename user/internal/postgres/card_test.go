package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestCardRepository(t *testing.T) {
	repo, pool := newRepo(t)
	cards := postgres.NewCardRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	number, err := user.RandomCardNumber()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)

	if _, err = cards.AddCard(ctx, uuid.Must(uuid.NewV7()), user.NewBankCard(number, now)); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("AddCard of nobody: %v", err)
	}

	ann, bob := savedUser(t, repo, "card-ann"), savedUser(t, repo, "card-bob")

	if _, err = cards.Card(ctx, ann); !errors.Is(err, user.ErrNoCard) {
		t.Fatalf("Card before issuing: %v", err)
	}
	if _, err = cards.SetCardCover(ctx, ann, "gold"); !errors.Is(err, user.ErrNoCard) {
		t.Fatalf("SetCardCover before issuing: %v", err)
	}

	got, err := cards.AddCard(ctx, ann, user.NewBankCard(number, now))
	if err != nil || got.Number() != number || got.Cover() != user.DefaultCardCover || !got.IssuedAt().Equal(now) {
		t.Fatalf("AddCard = %+v, %v", got, err)
	}

	// The second card of a page leaves the first.
	other, err := user.RandomCardNumber()
	if err != nil {
		t.Fatal(err)
	}
	if got, err = cards.AddCard(ctx, ann, user.NewBankCard(other, now.Add(time.Hour))); err != nil || got.Number() != number {
		t.Fatalf("AddCard again = %+v, %v", got, err)
	}

	if _, err = cards.AddCard(ctx, bob, user.NewBankCard(number, now)); !errors.Is(err, user.ErrCardNumberTaken) {
		t.Fatalf("AddCard of a taken number: %v", err)
	}

	if got, err = cards.SetCardCover(ctx, ann, "gold"); err != nil || got.Cover() != "gold" {
		t.Fatalf("SetCardCover = %+v, %v", got, err)
	}
	if got, err = cards.Card(ctx, ann); err != nil || got.Cover() != "gold" || got.Number() != number {
		t.Fatalf("Card = %+v, %v", got, err)
	}
}
