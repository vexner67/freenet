package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// takenCards answers user.ErrCardNumberTaken a few times, then as the
// repository it wraps; or always, with an endless count.
type takenCards struct {
	*inmem.CardRepository
	taken int
}

func (r *takenCards) AddCard(ctx context.Context, userID uuid.UUID, c user.BankCard) (user.BankCard, error) {
	if r.taken > 0 {
		r.taken--
		return user.BankCard{}, user.ErrCardNumberTaken
	}

	return r.CardRepository.AddCard(ctx, userID, c)
}

func TestCardService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now().UTC()
	ann, bob := uuid.New(), uuid.New()
	cards := service.NewCardService(inmem.NewCardRepository())

	if _, err := cards.Card(ctx, ann); !errors.Is(err, user.ErrNoCard) {
		t.Fatalf("Card before issuing: %v", err)
	}
	if _, err := cards.SetCover(ctx, ann, "gold"); !errors.Is(err, user.ErrNoCard) {
		t.Fatalf("SetCover before issuing: %v", err)
	}

	first, err := cards.Issue(ctx, ann, now)
	if err != nil || first.Cover() != user.DefaultCardCover {
		t.Fatalf("Issue = %+v, %v", first, err)
	}

	// A page has one card, whoever asks twice.
	again, err := cards.Issue(ctx, ann, now.Add(time.Hour))
	if err != nil || again.Number() != first.Number() || !again.IssuedAt().Equal(now) {
		t.Fatalf("Issue again = %+v, %v", again, err)
	}

	other, err := cards.Issue(ctx, bob, now)
	if err != nil || other.Number() == first.Number() {
		t.Fatalf("Issue of bob = %+v, %v", other, err)
	}

	changed, err := cards.SetCover(ctx, ann, "gold")
	if err != nil || changed.Cover() != "gold" || changed.Number() != first.Number() {
		t.Fatalf("SetCover = %+v, %v", changed, err)
	}
	if got, err := cards.Card(ctx, ann); err != nil || got.Cover() != "gold" {
		t.Fatalf("Card = %+v, %v", got, err)
	}
}

func TestCardServiceRetriesATakenNumber(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := &takenCards{CardRepository: inmem.NewCardRepository(), taken: user.CardNumberTries - 1}

	if _, err := service.NewCardService(repo).Issue(ctx, uuid.New(), time.Now()); err != nil {
		t.Fatalf("Issue after taken numbers: %v", err)
	}

	repo.taken = user.CardNumberTries
	if _, err := service.NewCardService(repo).Issue(ctx, uuid.New(), time.Now()); !errors.Is(err, user.ErrCardNumberTaken) {
		t.Fatalf("Issue with every number taken: %v", err)
	}
}
