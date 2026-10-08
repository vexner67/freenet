package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// FollowChecker tells whether one user follows another.
type FollowChecker interface {
	IsFollowing(ctx context.Context, followerID, followeeID uuid.UUID) (bool, error)
}

// PermissionService answers what one user may do to another: write on the
// wall, write messages. The rules are the owner's (user.User); this
// gathers the ties they depend on, follows and blocks, which live here.
type PermissionService struct {
	userRepo   UserRepository
	followRepo FollowChecker
	blockRepo  BlockChecker
}

func NewPermissionService(userRepo UserRepository, followRepo FollowChecker, blockRepo BlockChecker) *PermissionService {
	return &PermissionService{
		userRepo:   userRepo,
		followRepo: followRepo,
		blockRepo:  blockRepo,
	}
}

// CanPostOnWall tells whether authorID may write on ownerID's wall.
// user.ErrNotFound for an unknown owner.
func (s *PermissionService) CanPostOnWall(ctx context.Context, authorID, ownerID uuid.UUID) (bool, error) {
	owner, ties, err := s.ties(ctx, authorID, ownerID)
	if err != nil {
		return false, errs.Wrap("s.ties", err)
	}

	return owner.CanPostOnWall(authorID, ties), nil
}

// CanMessage tells whether senderID may write to recipientID; wroteFirst
// is whether the recipient has written to the sender, which the message
// service knows. user.ErrNotFound for an unknown recipient.
func (s *PermissionService) CanMessage(ctx context.Context, senderID, recipientID uuid.UUID, wroteFirst bool) (bool, error) {
	recipient, ties, err := s.ties(ctx, senderID, recipientID)
	if err != nil {
		return false, errs.Wrap("s.ties", err)
	}

	ties.WroteFirst = wroteFirst

	return recipient.CanBeMessagedBy(senderID, ties), nil
}

// CanSeeClubs tells whether viewerID (uuid.Nil: anonymous) may see which
// communities ownerID is in. user.ErrNotFound for an unknown owner.
func (s *PermissionService) CanSeeClubs(ctx context.Context, viewerID, ownerID uuid.UUID) (bool, error) {
	owner, ties, err := s.ties(ctx, viewerID, ownerID)
	if err != nil {
		return false, errs.Wrap("s.ties", err)
	}

	return owner.ClubsVisibleTo(viewerID, ties), nil
}

// ties loads ownerID and how it is tied to otherID.
func (s *PermissionService) ties(ctx context.Context, otherID, ownerID uuid.UUID) (*user.User, user.Ties, error) {
	owner, err := s.userRepo.GetByID(ctx, ownerID)
	if err != nil {
		return nil, user.Ties{}, errs.Wrap("s.userRepo.GetByID", err)
	}

	followed, err := s.followRepo.IsFollowing(ctx, ownerID, otherID)
	if err != nil {
		return nil, user.Ties{}, errs.Wrap("s.followRepo.IsFollowing", err)
	}

	byOwner, byOther, err := s.blockRepo.Blocks(ctx, ownerID, otherID)
	if err != nil {
		return nil, user.Ties{}, errs.Wrap("s.blockRepo.Blocks", err)
	}

	return owner, user.Ties{Followed: followed, Blocked: byOwner || byOther}, nil
}
