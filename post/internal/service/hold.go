package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// ReactionHolder keeps the likes and the poll votes of a user aside while
// their page is deleted: Hold takes them out of the counts and the lists
// of who liked, Release puts back those whose target is still there, and
// Forget drops what is held of a user erased for good. All three are
// idempotent.
type ReactionHolder interface {
	Hold(ctx context.Context, userID uuid.UUID) error
	Release(ctx context.Context, userID uuid.UUID) error
	Forget(ctx context.Context, userID uuid.UUID) error
}

// HoldService hides what a deleted page liked and voted for until the
// page is restored: a page nobody can see should not be counted either.
type HoldService struct {
	holders []ReactionHolder
	tx      TxManager
}

func NewHoldService(tx TxManager, holders ...ReactionHolder) *HoldService {
	return &HoldService{
		holders: holders,
		tx:      tx,
	}
}

// HoldUser takes the user's likes and votes out of the counts.
func (s *HoldService) HoldUser(ctx context.Context, userID uuid.UUID) error {
	return s.each(ctx, func(ctx context.Context, h ReactionHolder) error {
		return errs.Wrap("h.Hold", h.Hold(ctx, userID))
	})
}

// ReleaseUser counts them again.
func (s *HoldService) ReleaseUser(ctx context.Context, userID uuid.UUID) error {
	return s.each(ctx, func(ctx context.Context, h ReactionHolder) error {
		return errs.Wrap("h.Release", h.Release(ctx, userID))
	})
}

func (s *HoldService) each(ctx context.Context, fn func(ctx context.Context, h ReactionHolder) error) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		for _, h := range s.holders {
			if err := fn(ctx, h); err != nil {
				return err
			}
		}

		return nil
	})

	return errs.Wrap("s.tx.WithinTx", err)
}
