package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// CardRepository keeps the virtual cards of pages.
type CardRepository interface {
	// Card is user.ErrNoCard for a page without one.
	Card(ctx context.Context, userID uuid.UUID) (user.BankCard, error)
	// AddCard keeps c unless the page has a card, and returns the card the
	// page has. user.ErrCardNumberTaken, user.ErrNotFound.
	AddCard(ctx context.Context, userID uuid.UUID, c user.BankCard) (user.BankCard, error)
	// SetCardCover is user.ErrNoCard for a page without a card.
	SetCardCover(ctx context.Context, userID uuid.UUID, cover user.CardCover) (user.BankCard, error)
}

// CardService issues the virtual cards of the bank and changes their
// covers.
type CardService struct {
	cards CardRepository
}

func NewCardService(cards CardRepository) *CardService {
	return &CardService{cards: cards}
}

// Card is the card of userID; user.ErrNoCard.
func (s *CardService) Card(ctx context.Context, userID uuid.UUID) (user.BankCard, error) {
	c, err := s.cards.Card(ctx, userID)
	if err != nil {
		return user.BankCard{}, errs.Wrap("s.cards.Card", err)
	}

	return c, nil
}

// Issue gives userID a card with a number of its own at now. A page has
// one card: asked again, it gets the card it has.
func (s *CardService) Issue(ctx context.Context, userID uuid.UUID, now time.Time) (user.BankCard, error) {
	var last error

	for range user.CardNumberTries {
		number, err := user.RandomCardNumber()
		if err != nil {
			return user.BankCard{}, errs.Wrap("user.RandomCardNumber", err)
		}

		c, err := s.cards.AddCard(ctx, userID, user.NewBankCard(number, now))
		if err == nil {
			return c, nil
		}
		if !errors.Is(err, user.ErrCardNumberTaken) {
			return user.BankCard{}, errs.Wrap("s.cards.AddCard", err)
		}
		last = err
	}

	return user.BankCard{}, errs.Wrap("s.cards.AddCard", last)
}

// SetCover changes the cover of the card of userID; user.ErrNoCard.
func (s *CardService) SetCover(ctx context.Context, userID uuid.UUID, cover user.CardCover) (user.BankCard, error) {
	c, err := s.cards.SetCardCover(ctx, userID, cover)
	if err != nil {
		return user.BankCard{}, errs.Wrap("s.cards.SetCardCover", err)
	}

	return c, nil
}
