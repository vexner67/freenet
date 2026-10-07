package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// Edge is one follow: Follower follows Followee.
type Edge struct {
	Follower uuid.UUID
	Followee uuid.UUID
}

// BlockChecker tells about blocks between two users.
type BlockChecker interface {
	// Blocks tells whether a blacklisted b and whether b blacklisted a.
	Blocks(ctx context.Context, a, b uuid.UUID) (aBlockedB, bBlockedA bool, err error)
}

type BlockRepository interface {
	BlockChecker
	// Block puts blocked into blocker's blacklist and drops the follows
	// between the two either way, in one step; it returns the dropped
	// follows. Blocking twice is not an error.
	Block(ctx context.Context, blockerID, blockedID uuid.UUID) ([]Edge, error)
	// Unblock is idempotent.
	Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error
	// ListBlocked returns up to limit blacklisted users, newest block
	// first, with block IDs (UUIDv7) below before (uuid.Nil: the newest).
	ListBlocked(ctx context.Context, blockerID, before uuid.UUID, limit int) ([]Blocked, error)
	CountBlocked(ctx context.Context, blockerID uuid.UUID) (int, error)
}

// Blocked is a user in a blacklist with the ID of that block.
type Blocked struct {
	BlockID uuid.UUID
	User    *user.User
}

// BlockPage is one screen of a blacklist; NextCursor is uuid.Nil on the last.
type BlockPage struct {
	Users      []user.PublicProfile
	NextCursor uuid.UUID
	Total      int
}

// Relation is how a viewer and a user stand: who blacklisted whom.
type Relation struct {
	BlockedByViewer bool
	BlockedViewer   bool
}

// BlockService is the blacklist ("Чёрный список" on openvk): a block shuts
// messages, wall posts and follows between the two, both ways.
type BlockService struct {
	userRepo  UserRepository
	blockRepo BlockRepository
	notifier  Notifier
}

func NewBlockService(userRepo UserRepository, blockRepo BlockRepository, notifier Notifier) *BlockService {
	return &BlockService{
		userRepo:  userRepo,
		blockRepo: blockRepo,
		notifier:  notifier,
	}
}

// Block blacklists blockedID and drops the follows between the two, as
// openvk does; their notifications go too.
func (s *BlockService) Block(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if blockerID == blockedID {
		return user.ErrBlockSelf
	}

	if _, err := s.userRepo.GetByID(ctx, blockedID); err != nil {
		return errs.Wrap("s.userRepo.GetByID", err)
	}

	dropped, err := s.blockRepo.Block(ctx, blockerID, blockedID)
	if err != nil {
		return errs.Wrap("s.blockRepo.Block", err)
	}

	for _, e := range dropped {
		s.notifier.Unfollowed(ctx, e.Follower, e.Followee)
	}

	return nil
}

func (s *BlockService) Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if err := s.blockRepo.Unblock(ctx, blockerID, blockedID); err != nil {
		return errs.Wrap("s.blockRepo.Unblock", err)
	}

	return nil
}

// List pages a blacklist, newest block first.
func (s *BlockService) List(ctx context.Context, blockerID, cursor uuid.UUID, size int) (BlockPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra row tells whether there is a next page.
	found, err := s.blockRepo.ListBlocked(ctx, blockerID, cursor, size+1)
	if err != nil {
		return BlockPage{}, errs.Wrap("s.blockRepo.ListBlocked", err)
	}

	total, err := s.blockRepo.CountBlocked(ctx, blockerID)
	if err != nil {
		return BlockPage{}, errs.Wrap("s.blockRepo.CountBlocked", err)
	}

	page := BlockPage{Total: total}

	if len(found) > size {
		found = found[:size]
		page.NextCursor = found[size-1].BlockID
	}

	page.Users = make([]user.PublicProfile, 0, len(found))
	for _, b := range found {
		page.Users = append(page.Users, b.User.PublicProfile())
	}

	return page, nil
}

// Relation tells who of viewerID and userID blacklisted the other.
func (s *BlockService) Relation(ctx context.Context, viewerID, userID uuid.UUID) (Relation, error) {
	byViewer, ofViewer, err := s.blockRepo.Blocks(ctx, viewerID, userID)
	if err != nil {
		return Relation{}, errs.Wrap("s.blockRepo.Blocks", err)
	}

	return Relation{BlockedByViewer: byViewer, BlockedViewer: ofViewer}, nil
}
