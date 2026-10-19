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

// VerificationService is openvk's "Верификация": the administrators of
// the site confirm a page, which then wears a checkmark by its name.
type VerificationService struct {
	userRepo      UserRepository
	verifications VerificationRepository
	admins        user.Admins
}

func NewVerificationService(users UserRepository, verifications VerificationRepository, admins user.Admins) *VerificationService {
	return &VerificationService{userRepo: users, verifications: verifications, admins: admins}
}

// Set gives userID the checkmark or takes it; user.ErrNotAdmin unless
// adminID runs the site, user.ErrNotFound for an unknown page.
func (s *VerificationService) Set(ctx context.Context, adminID, userID uuid.UUID, verified bool) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.verifications.SetVerified", s.verifications.SetVerified(ctx, userID, verified))
}
