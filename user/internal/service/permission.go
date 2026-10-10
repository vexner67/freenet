package service

import (
	"context"
	"errors"
	"slices"
	"time"

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
	now        func() time.Time
}

func NewPermissionService(userRepo UserRepository, followRepo FollowChecker, blockRepo BlockChecker) *PermissionService {
	return &PermissionService{
		userRepo:   userRepo,
		followRepo: followRepo,
		blockRepo:  blockRepo,
		now:        time.Now,
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

// Access tells what of ownerID's page viewerID (uuid.Nil: anonymous) may
// see: the page, its information, albums, follows and communities.
// user.ErrNotFound for an unknown owner.
func (s *PermissionService) Access(ctx context.Context, viewerID, ownerID uuid.UUID) (user.Access, error) {
	owner, ties, err := s.ties(ctx, viewerID, ownerID)
	if err != nil {
		return user.Access{}, errs.Wrap("s.ties", err)
	}

	return owner.AccessFor(viewerID, ties), nil
}

// ProfileFor is the page of nickname as viewerID (uuid.Nil: anonymous)
// sees it. user.ErrNotFound for nobody.
func (s *PermissionService) ProfileFor(ctx context.Context, viewerID uuid.UUID, nickname user.Nickname) (user.PublicProfile, error) {
	owner, err := s.userRepo.GetByNickname(ctx, nickname)
	if err != nil {
		return user.PublicProfile{}, errs.Wrap("s.userRepo.GetByNickname", err)
	}

	ties, err := s.tiesOf(ctx, viewerID, owner.ID())
	if err != nil {
		return user.PublicProfile{}, errs.Wrap("s.tiesOf", err)
	}

	return owner.PublicProfileFor(viewerID, ties, s.now()), nil
}

// Mentioned are the people authorID named by nickname who hear of it, in
// the order named: those who exist and may be named by the author
// (user.CanBeMentionedBy). At most user.MaxMentions nicknames are looked
// at; an unknown one names nobody.
func (s *PermissionService) Mentioned(ctx context.Context, authorID uuid.UUID, nicknames []user.Nickname) ([]uuid.UUID, error) {
	if len(nicknames) > user.MaxMentions {
		nicknames = nicknames[:user.MaxMentions]
	}

	var mentioned []uuid.UUID

	for _, nickname := range nicknames {
		u, err := s.userRepo.GetByNickname(ctx, nickname)
		if errors.Is(err, user.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, errs.Wrap("s.userRepo.GetByNickname", err)
		}

		if slices.Contains(mentioned, u.ID()) {
			continue
		}

		ties, err := s.tiesOf(ctx, authorID, u.ID())
		if err != nil {
			return nil, errs.Wrap("s.tiesOf", err)
		}

		if u.CanBeMentionedBy(authorID, ties) {
			mentioned = append(mentioned, u.ID())
		}
	}

	return mentioned, nil
}

// ties loads ownerID and how it is tied to otherID.
func (s *PermissionService) ties(ctx context.Context, otherID, ownerID uuid.UUID) (*user.User, user.Ties, error) {
	owner, err := s.userRepo.GetByID(ctx, ownerID)
	if err != nil {
		return nil, user.Ties{}, errs.Wrap("s.userRepo.GetByID", err)
	}

	ties, err := s.tiesOf(ctx, otherID, ownerID)
	if err != nil {
		return nil, user.Ties{}, errs.Wrap("s.tiesOf", err)
	}

	return owner, ties, nil
}

// tiesOf is how ownerID is tied to otherID; none to an anonymous other.
func (s *PermissionService) tiesOf(ctx context.Context, otherID, ownerID uuid.UUID) (user.Ties, error) {
	if otherID == uuid.Nil {
		return user.Ties{}, nil
	}

	followed, err := s.followRepo.IsFollowing(ctx, ownerID, otherID)
	if err != nil {
		return user.Ties{}, errs.Wrap("s.followRepo.IsFollowing", err)
	}

	byOwner, byOther, err := s.blockRepo.Blocks(ctx, ownerID, otherID)
	if err != nil {
		return user.Ties{}, errs.Wrap("s.blockRepo.Blocks", err)
	}

	return user.Ties{Followed: followed, Blocked: byOwner || byOther}, nil
}
