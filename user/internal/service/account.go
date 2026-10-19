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
	userRepo  UserRepository
	tx        TxManager
	reactions Reactions
	keepFor   time.Duration
	now       func() time.Time
}

// Reactions records, in the transaction that deletes or restores a page,
// that its likes and poll votes stop or start counting again: the service
// that keeps them sets them aside and puts them back afterwards (outbox).
type Reactions interface {
	UserDeactivated(ctx context.Context, userID uuid.UUID) error
	UserReactivated(ctx context.Context, userID uuid.UUID) error
}

func NewAccountService(userRepo UserRepository, tx TxManager, reactions Reactions, keepFor time.Duration) *AccountService {
	return &AccountService{
		userRepo:  userRepo,
		tx:        tx,
		reactions: reactions,
		keepFor:   keepFor,
		now:       time.Now,
	}
}

// Deactivate deletes the page of id; user.ErrDeactivated if it is already.
func (s *AccountService) Deactivate(ctx context.Context, id uuid.UUID, reason user.DeactivationReason) (*user.User, error) {
	return s.change(ctx, id, s.reactions.UserDeactivated, func(u *user.User) error {
		return errs.Wrap("u.Deactivate", u.Deactivate(reason, s.now().UTC(), s.keepFor))
	})
}

// Reactivate restores the page of id: user.ErrNotDeactivated if it is not
// deleted, user.ErrRestoreExpired once it is due to be erased.
func (s *AccountService) Reactivate(ctx context.Context, id uuid.UUID) (*user.User, error) {
	return s.change(ctx, id, s.reactions.UserReactivated, func(u *user.User) error {
		return errs.Wrap("u.Reactivate", u.Reactivate(s.now()))
	})
}

// AccountState is what the access tokens of a page may do.
type AccountState struct {
	// Deactivated: the page is deleted; its tokens are good only to
	// restore it.
	Deactivated bool
	// Banned: the administrators blocked the page; its tokens are good
	// only to read why.
	Banned bool
}

// State tells the auth service which access tokens to issue.
func (s *AccountService) State(ctx context.Context, id uuid.UUID) (AccountState, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return AccountState{}, errs.Wrap("s.userRepo.GetByID", err)
	}

	return AccountState{Deactivated: u.IsDeactivated(), Banned: u.IsBannedAt(s.now())}, nil
}

// change saves the page as fn left it and records the change by record,
// both or neither.
func (s *AccountService) change(
	ctx context.Context,
	id uuid.UUID,
	record func(context.Context, uuid.UUID) error,
	fn func(*user.User) error,
) (*user.User, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByID", err)
	}

	if err = fn(u); err != nil {
		return nil, err
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.userRepo.Update(ctx, u); err != nil {
			return errs.Wrap("s.userRepo.Update", err)
		}

		return errs.Wrap("record", record(ctx, id))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return u, nil
}
