package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// RoomAccess contains relationships only; private profile fields never cross
// the boundary into the room service.
type RoomAccess struct{ Friend, Blocked, Banned, CanInvite bool }

func (s *PermissionService) RoomAccess(ctx context.Context, ownerID, viewerID uuid.UUID) (RoomAccess, error) {
	owner, ties, err := s.ties(ctx, viewerID, ownerID)
	if err != nil {
		return RoomAccess{}, errs.Wrap("s.ties", err)
	}
	viewer, err := s.userRepo.GetByID(ctx, viewerID)
	if err != nil {
		return RoomAccess{}, errs.Wrap("s.userRepo.GetByID", err)
	}
	now := s.now()
	return RoomAccess{Friend: ties.Followed, Blocked: ties.Blocked || owner.IsDeactivated() || owner.IsBannedAt(now), Banned: viewer.IsDeactivated() || viewer.IsBannedAt(now), CanInvite: owner.CanBeInvitedBy(viewerID, ties)}, nil
}
