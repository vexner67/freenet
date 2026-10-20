package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// ForeignAgentRepository puts and takes the plaque of a page;
// user.ErrNotFound for an unknown one.
type ForeignAgentRepository interface {
	SetForeignAgent(ctx context.Context, userID uuid.UUID, agent bool) error
}

// ForeignAgentService is the orange plaque of openvk ("user-alert") the
// administrators of the site put on a page: "иностранный агент". openvk's
// plaque says what its administrators write into the table; ours says
// one thing and is put and taken in the admin panel. Nobody hears of it,
// as on openvk.
type ForeignAgentService struct {
	userRepo UserRepository
	agents   ForeignAgentRepository
	admins   user.Admins
}

func NewForeignAgentService(users UserRepository, agents ForeignAgentRepository, admins user.Admins) *ForeignAgentService {
	return &ForeignAgentService{userRepo: users, agents: agents, admins: admins}
}

// Set puts the plaque on userID or takes it; user.ErrNotAdmin unless
// adminID runs the site, user.ErrNotFound for an unknown page.
func (s *ForeignAgentService) Set(ctx context.Context, adminID, userID uuid.UUID, agent bool) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.agents.SetForeignAgent", s.agents.SetForeignAgent(ctx, userID, agent))
}
