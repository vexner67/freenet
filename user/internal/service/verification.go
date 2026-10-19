package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// VerificationRepository gives and takes the checkmark of a page;
// user.ErrNotFound for an unknown one.
type VerificationRepository interface {
	SetVerified(ctx context.Context, userID uuid.UUID, verified bool) error
}

// VerifiedNotifier records what the notification service has to hear
// about checkmarks, in the transaction that gives or takes one, as
// Notifier does.
type VerifiedNotifier interface {
	// Verified: adminID gave userID the checkmark.
	Verified(ctx context.Context, adminID, userID uuid.UUID) error
	// Unverified takes back the notification of it, whoever gave it.
	Unverified(ctx context.Context, adminID, userID uuid.UUID) error
}

// VerificationService is openvk's "Верификация": the administrators of
// the site confirm a page, which then wears a checkmark by its name. Its
// owner hears of it; openvk tells nobody.
type VerificationService struct {
	userRepo      UserRepository
	verifications VerificationRepository
	tx            TxManager
	notifier      VerifiedNotifier
	admins        user.Admins
}

// VerificationDeps are what VerificationService works with.
type VerificationDeps struct {
	Users         UserRepository
	Verifications VerificationRepository
	Tx            TxManager
	Notifier      VerifiedNotifier
	Admins        user.Admins
}

func NewVerificationService(d VerificationDeps) *VerificationService {
	return &VerificationService{
		userRepo:      d.Users,
		verifications: d.Verifications,
		tx:            d.Tx,
		notifier:      d.Notifier,
		admins:        d.Admins,
	}
}

// Set gives userID the checkmark or takes it, and the owner of the page
// hears of it (an administrator confirming their own page does not);
// user.ErrNotAdmin unless adminID runs the site, user.ErrNotFound for an
// unknown page.
func (s *VerificationService) Set(ctx context.Context, adminID, userID uuid.UUID, verified bool) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.verifications.SetVerified(ctx, userID, verified); err != nil {
			return errs.Wrap("s.verifications.SetVerified", err)
		}

		switch {
		case adminID == userID:
			return nil
		case verified:
			return errs.Wrap("s.notifier.Verified", s.notifier.Verified(ctx, adminID, userID))
		default:
			return errs.Wrap("s.notifier.Unverified", s.notifier.Unverified(ctx, adminID, userID))
		}
	})

	return errs.Wrap("s.tx.WithinTx", err)
}
