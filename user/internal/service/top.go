package service

import (
	"context"
	"time"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// TopRepository counts the boards of "Рейтинги" among pages neither
// deleted nor banned at now, the greatest first, only above zero.
type TopRepository interface {
	// Rich are those who hold the most votes.
	Rich(ctx context.Context, now time.Time, limit int) ([]user.TopEntry, error)
	// Earned are those whose lines of kinds since add up to the most.
	Earned(ctx context.Context, kinds []user.VoteKind, since, now time.Time, limit int) ([]user.TopEntry, error)
	// Pets are those whose pets have the highest rating.
	Pets(ctx context.Context, now time.Time, limit int) ([]user.TopEntry, error)
}

// TopService is "Рейтинги": the richest, the best miners, businessmen and
// players of the week and the strongest pets.
type TopService struct {
	tops TopRepository
}

func NewTopService(tops TopRepository) *TopService {
	return &TopService{tops: tops}
}

// Top is board at now.
func (s *TopService) Top(ctx context.Context, board user.TopBoard, now time.Time) ([]user.TopEntry, error) {
	switch {
	case board == user.TopRich:
		entries, err := s.tops.Rich(ctx, now, user.MaxTop)

		return entries, errs.Wrap("s.tops.Rich", err)
	case board == user.TopPets:
		entries, err := s.tops.Pets(ctx, now, user.MaxTop)

		return entries, errs.Wrap("s.tops.Pets", err)
	case board.Weekly():
		entries, err := s.tops.Earned(ctx, board.Kinds(), now.Add(-user.TopWeek), now, user.MaxTop)

		return entries, errs.Wrap("s.tops.Earned", err)
	default:
		return nil, user.ErrUnknownTop
	}
}
