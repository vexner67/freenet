package service

import (
	"context"

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
}

func NewBackdropService(backdrops BackdropRepository, users UserRepository) *BackdropService {
	return &BackdropService{backdrops: backdrops, users: users}
}

// Set shows photoID (uuid.Nil: none) on a side of id's page; the photo
// the side showed before is the caller's to delete.
func (s *BackdropService) Set(ctx context.Context, id uuid.UUID, side user.BackdropSide, photoID uuid.UUID) (*user.User, uuid.UUID, error) {
	previous, err := s.backdrops.SetBackdrop(ctx, id, side, photoID)
	if err != nil {
		return nil, uuid.Nil, errs.Wrap("s.backdrops.SetBackdrop", err)
	}

	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, uuid.Nil, errs.Wrap("s.users.GetByID", err)
	}

	return u, previous, nil
}
