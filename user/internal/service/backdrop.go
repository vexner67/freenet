package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// BackdropRepository keeps the backdrops of pages.
type BackdropRepository interface {
	// SetBackdrop shows photoID (uuid.Nil: none) on a side of id's page
	// and returns what it showed before, in one statement, so that two
	// changes of a side tell each the photo it replaced; user.ErrNotFound.
	SetBackdrop(ctx context.Context, id uuid.UUID, side user.BackdropSide, photoID uuid.UUID) (uuid.UUID, error)
}

// BackdropService sets openvk's "Фон страницы" of people.
type BackdropService struct {
	backdrops BackdropRepository
	users     UserRepository
	now       func() time.Time
}

func NewBackdropService(backdrops BackdropRepository, users UserRepository) *BackdropService {
	return &BackdropService{backdrops: backdrops, users: users, now: time.Now}
}

// Set shows photoID (uuid.Nil: none) on a side of id's page; the photo
// the side showed before is the caller's to delete. A photo behind the
// whole page needs Prime (user.ErrPrimeOnly).
func (s *BackdropService) Set(ctx context.Context, id uuid.UUID, side user.BackdropSide, photoID uuid.UUID) (*user.User, uuid.UUID, error) {
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, uuid.Nil, errs.Wrap("s.users.GetByID", err)
	}

	if err = u.CheckBackdrop(side, photoID, s.now()); err != nil {
		return nil, uuid.Nil, errs.Wrap("u.CheckBackdrop", err)
	}

	previous, err := s.backdrops.SetBackdrop(ctx, id, side, photoID)
	if err != nil {
		return nil, uuid.Nil, errs.Wrap("s.backdrops.SetBackdrop", err)
	}

	if u, err = s.users.GetByID(ctx, id); err != nil {
		return nil, uuid.Nil, errs.Wrap("s.users.GetByID", err)
	}

	return u, previous, nil
}
