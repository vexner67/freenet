package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// DecorRepository keeps how the site is dressed for the New Year.
type DecorRepository interface {
	// Decor is user.DecorAuto until one is set.
	Decor(ctx context.Context) (user.Decor, error)
	SetDecor(ctx context.Context, decor user.Decor) error
}

// DecorService is the New Year decoration of the site: the
// administrators say whether it is on, and everyone asks whether to wear
// it now.
type DecorService struct {
	userRepo UserRepository
	decors   DecorRepository
	admins   user.Admins
	now      func() time.Time
}

func NewDecorService(users UserRepository, decors DecorRepository, admins user.Admins) *DecorService {
	return &DecorService{userRepo: users, decors: decors, admins: admins, now: time.Now}
}

// Decor is the setting and whether the site wears the decoration now;
// anyone may ask.
func (s *DecorService) Decor(ctx context.Context) (user.Decor, bool, error) {
	decor, err := s.decors.Decor(ctx)
	if err != nil {
		return "", false, errs.Wrap("s.decors.Decor", err)
	}

	return decor, decor.FestiveAt(s.now()), nil
}

// SetDecor dresses the site from now on; user.ErrNotAdmin.
func (s *DecorService) SetDecor(ctx context.Context, adminID uuid.UUID, decor user.Decor) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.decors.SetDecor", s.decors.SetDecor(ctx, decor))
}
