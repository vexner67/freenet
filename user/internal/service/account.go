package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// AccountService deletes and restores pages, as openvk's "Удаление
// страницы": a deleted page is hidden at once and erased once its owner
// has not restored it for KeepFor.
type AccountService struct {
	userRepo UserRepository
	keepFor  time.Duration
	now      func() time.Time
}

func NewAccountService(userRepo UserRepository, keepFor time.Duration) *AccountService {
	return &AccountService{
		userRepo: userRepo,
		keepFor:  keepFor,
		now:      time.Now,
	}
}

// Deactivate deletes the page of id; user.ErrDeactivated if it is already.
func (s *AccountService) Deactivate(ctx context.Context, id uuid.UUID, reason user.DeactivationReason) (*user.User, error) {
	return s.change(ctx, id, func(u *user.User) error {
		return errs.Wrap("u.Deactivate", u.Deactivate(reason, s.now().UTC(), s.keepFor))
	})
}

// Reactivate restores the page of id: user.ErrNotDeactivated if it is not
// deleted, user.ErrRestoreExpired once it is due to be erased.
func (s *AccountService) Reactivate(ctx context.Context, id uuid.UUID) (*user.User, error) {
	return s.change(ctx, id, func(u *user.User) error {
		return errs.Wrap("u.Reactivate", u.Reactivate(s.now()))
	})
}

// IsDeactivated tells the auth service which access tokens to issue:
// those of a deleted page are good only to restore it.
func (s *AccountService) IsDeactivated(ctx context.Context, id uuid.UUID) (bool, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return false, errs.Wrap("s.userRepo.GetByID", err)
	}

	return u.IsDeactivated(), nil
}

func (s *AccountService) change(ctx context.Context, id uuid.UUID, fn func(*user.User) error) (*user.User, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByID", err)
	}

	if err = fn(u); err != nil {
		return nil, err
	}

	if err = s.userRepo.Update(ctx, u); err != nil {
		return nil, errs.Wrap("s.userRepo.Update", err)
	}

	return u, nil
}
