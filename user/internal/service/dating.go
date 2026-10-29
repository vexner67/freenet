package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// DatingRepository keeps the cards of "Знакомства" and who liked or skipped
// whom.
type DatingRepository interface {
	// Card: user.ErrNotFound for a person who never made one.
	Card(ctx context.Context, userID uuid.UUID) (user.DatingCard, error)
	SaveCard(ctx context.Context, c user.DatingCard) error
	// Candidates are up to limit cards that are on, not of userID and not
	// answered by them yet, the most recently changed first.
	Candidates(ctx context.Context, userID uuid.UUID, limit int) ([]user.DatingCard, error)
	// Answer records that fromID liked or skipped toID at at;
	// user.ErrDatingTwice when they have answered before.
	Answer(ctx context.Context, fromID, toID uuid.UUID, like bool, at time.Time) error
	// Liked tells whether fromID liked toID.
	Liked(ctx context.Context, fromID, toID uuid.UUID) (bool, error)
	// Matches are who userID and who liked each other, the latest first.
	Matches(ctx context.Context, userID uuid.UUID) ([]user.DatingMatch, error)
}

// DatingNotifier records, in the transaction of the second like, that two
// people liked each other, as Notifier does.
type DatingNotifier interface {
	Matched(ctx context.Context, firstID, secondID uuid.UUID) error
}

// datingLook is how many candidates are read for one deck: some of them
// are not shown (the gender does not suit, the page is deleted or banned).
const datingLook = 200

// DatingView is "Знакомства" for one person: their card (nil before they
// made one), the cards to answer and the matches.
type DatingView struct {
	Card    *user.DatingCard
	Deck    []user.DatingCard
	Matches []user.DatingMatch
}

// DatingAnswer is a like or a skip of a card.
type DatingAnswer struct {
	UserID   uuid.UUID
	TargetID uuid.UUID
	Like     bool
	Now      time.Time
}

// DatingService is "Знакомства": people show cards to each other, like or
// skip them, and a like answered with a like is a match.
type DatingService struct {
	users    UserRepository
	dating   DatingRepository
	notifier DatingNotifier
	tx       TxManager
}

func NewDatingService(users UserRepository, dating DatingRepository, notifier DatingNotifier, tx TxManager) *DatingService {
	return &DatingService{users: users, dating: dating, notifier: notifier, tx: tx}
}

func (s *DatingService) Dating(ctx context.Context, userID uuid.UUID, now time.Time) (DatingView, error) {
	var view DatingView

	card, err := s.dating.Card(ctx, userID)
	if err != nil && !missing(err) {
		return DatingView{}, errs.Wrap("s.dating.Card", err)
	}
	if err == nil {
		view.Card = &card
		if view.Deck, err = s.deck(ctx, card, now); err != nil {
			return DatingView{}, err
		}
	}

	if view.Matches, err = s.dating.Matches(ctx, userID); err != nil {
		return DatingView{}, errs.Wrap("s.dating.Matches", err)
	}

	return view, nil
}

// deck are the cards card is shown: on, each accepting the gender of the
// other, of pages neither deleted nor banned.
func (s *DatingService) deck(ctx context.Context, card user.DatingCard, now time.Time) ([]user.DatingCard, error) {
	if !card.On {
		return nil, nil
	}

	me, err := s.users.GetByID(ctx, card.UserID)
	if err != nil {
		return nil, errs.Wrap("s.users.GetByID", err)
	}

	cards, err := s.dating.Candidates(ctx, card.UserID, datingLook)
	if err != nil {
		return nil, errs.Wrap("s.dating.Candidates", err)
	}

	ids := make([]uuid.UUID, len(cards))
	for i, c := range cards {
		ids[i] = c.UserID
	}
	people, err := s.users.GetByIDs(ctx, ids)
	if err != nil {
		return nil, errs.Wrap("s.users.GetByIDs", err)
	}
	byID := make(map[uuid.UUID]*user.User, len(people))
	for _, u := range people {
		byID[u.ID()] = u
	}

	deck := make([]user.DatingCard, 0, user.DatingDeckSize)
	for _, c := range cards {
		u, ok := byID[c.UserID]
		if !ok || !shownInDating(u, now) || !card.Suits(me.Profile().Gender, c, u.Profile().Gender) {
			continue
		}
		if deck = append(deck, c); len(deck) == user.DatingDeckSize {
			break
		}
	}

	return deck, nil
}

func shownInDating(u *user.User, now time.Time) bool {
	return u.IsProfileCompleted() && !u.IsDeactivated() && !u.IsBannedAt(now)
}

// SaveCard makes or changes the card of its owner; turned off, it hides
// them from the others and the others from them.
func (s *DatingService) SaveCard(ctx context.Context, c user.DatingCard) error {
	u, err := s.users.GetByID(ctx, c.UserID)
	if err != nil {
		return errs.Wrap("s.users.GetByID", err)
	}
	if !u.IsProfileCompleted() {
		return user.ErrDatingProfile
	}

	return errs.Wrap("s.dating.SaveCard", s.dating.SaveCard(ctx, c))
}

// Answer likes or skips a card the person was shown and tells whether the
// like made a match.
func (s *DatingService) Answer(ctx context.Context, a DatingAnswer) (bool, error) {
	if a.UserID == a.TargetID {
		return false, user.ErrDatingSelf
	}

	mine, err := s.dating.Card(ctx, a.UserID)
	if err != nil && !missing(err) {
		return false, errs.Wrap("s.dating.Card", err)
	}
	if err != nil || !mine.On {
		return false, user.ErrDatingOff
	}

	theirs, err := s.dating.Card(ctx, a.TargetID)
	if missing(err) {
		return false, user.ErrNotFound
	}
	if err != nil {
		return false, errs.Wrap("s.dating.Card", err)
	}
	if !theirs.On {
		return false, user.ErrNotFound
	}

	var matched bool

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.dating.Answer(ctx, a.UserID, a.TargetID, a.Like, a.Now); err != nil {
			return errs.Wrap("s.dating.Answer", err)
		}
		if !a.Like {
			return nil
		}

		liked, err := s.dating.Liked(ctx, a.TargetID, a.UserID)
		if err != nil || !liked {
			return errs.Wrap("s.dating.Liked", err)
		}
		matched = true

		return errs.Wrap("s.notifier.Matched", s.notifier.Matched(ctx, a.UserID, a.TargetID))
	})
	if err != nil {
		return false, errs.Wrap("s.tx.WithinTx", err)
	}

	return matched, nil
}
