package service

import (
	"context"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// EndlessRepository raises the balances of pages.
type EndlessRepository interface {
	// Endow raises to votes the balance of every page whose email is one
	// of emails (in lower case) and has less, and says how many it raised.
	Endow(ctx context.Context, emails []string, votes int64) (int, error)
}

// EndlessService keeps the votes of the administrators endless: whatever
// they spend comes back.
type EndlessService struct {
	repo   EndlessRepository
	admins user.Admins
}

func NewEndlessService(repo EndlessRepository, admins user.Admins) *EndlessService {
	return &EndlessService{repo: repo, admins: admins}
}

// Endow gives the administrators back user.EndlessVotes, and says to how
// many.
func (s *EndlessService) Endow(ctx context.Context) (int, error) {
	if len(s.admins) == 0 {
		return 0, nil
	}

	raised, err := s.repo.Endow(ctx, s.admins.Emails(), user.EndlessVotes)

	return raised, errs.Wrap("s.repo.Endow", err)
}
