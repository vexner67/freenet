package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 50

	// MaxFeedSources bounds how many followed users a news feed reads.
	MaxFeedSources = 1000
)

type FollowRepository interface {
	// Follow and Unfollow are idempotent.
	Follow(ctx context.Context, followerID, followeeID uuid.UUID) error
	Unfollow(ctx context.Context, followerID, followeeID uuid.UUID) error
	IsFollowing(ctx context.Context, followerID, followeeID uuid.UUID) (bool, error)
	Counts(ctx context.Context, userID uuid.UUID) (followers, following int, err error)
}

// FollowListRepository lists the people on one side of a user's follows,
// newest follow first. Cursors are follow IDs (UUIDv7).
type FollowListRepository interface {
	ListFollowers(ctx context.Context, userID, before uuid.UUID, limit int) ([]Followed, error)
	ListFollowing(ctx context.Context, userID, before uuid.UUID, limit int) ([]Followed, error)
	FollowingIDs(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error)
}

// Followed is a user in a follow list with the ID of that follow.
type Followed struct {
	FollowID uuid.UUID
	User     *user.User
}

type FollowStats struct {
	Followers int
	Following int
	// IsFollowing: the viewer follows the user; false for anonymous viewers.
	IsFollowing bool
}

// FollowPage is one screen of a follow list; NextCursor is uuid.Nil on the last.
type FollowPage struct {
	Users      []user.PublicProfile
	NextCursor uuid.UUID
	Total      int
}

type FollowService struct {
	userRepo   UserRepository
	followRepo FollowRepository
	listRepo   FollowListRepository
	notifier   Notifier
}

func NewFollowService(userRepo UserRepository, followRepo FollowRepository, listRepo FollowListRepository, notifier Notifier) *FollowService {
	return &FollowService{
		userRepo:   userRepo,
		followRepo: followRepo,
		listRepo:   listRepo,
		notifier:   notifier,
	}
}

func (s *FollowService) Follow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	if followerID == followeeID {
		return user.ErrFollowSelf
	}

	if _, err := s.userRepo.GetByID(ctx, followeeID); err != nil {
		return errs.Wrap("s.userRepo.GetByID", err)
	}

	if err := s.followRepo.Follow(ctx, followerID, followeeID); err != nil {
		return errs.Wrap("s.followRepo.Follow", err)
	}

	// Following twice notifies once: the notification service keeps one
	// notification per follow.
	s.notifier.Followed(ctx, followerID, followeeID)

	return nil
}

func (s *FollowService) Unfollow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	if err := s.followRepo.Unfollow(ctx, followerID, followeeID); err != nil {
		return errs.Wrap("s.followRepo.Unfollow", err)
	}

	if followerID != followeeID {
		s.notifier.Unfollowed(ctx, followerID, followeeID)
	}

	return nil
}

// Stats counts both sides of a user's follows; viewerID is uuid.Nil for an
// anonymous viewer.
func (s *FollowService) Stats(ctx context.Context, userID, viewerID uuid.UUID) (FollowStats, error) {
	followers, following, err := s.followRepo.Counts(ctx, userID)
	if err != nil {
		return FollowStats{}, errs.Wrap("s.followRepo.Counts", err)
	}

	stats := FollowStats{Followers: followers, Following: following}

	if viewerID != uuid.Nil && viewerID != userID {
		if stats.IsFollowing, err = s.followRepo.IsFollowing(ctx, viewerID, userID); err != nil {
			return FollowStats{}, errs.Wrap("s.followRepo.IsFollowing", err)
		}
	}

	return stats, nil
}

func (s *FollowService) ListFollowers(ctx context.Context, userID, cursor uuid.UUID, size int) (FollowPage, error) {
	return s.page(ctx, userID, cursor, size, s.listRepo.ListFollowers, func(followers, _ int) int { return followers })
}

func (s *FollowService) ListFollowing(ctx context.Context, userID, cursor uuid.UUID, size int) (FollowPage, error) {
	return s.page(ctx, userID, cursor, size, s.listRepo.ListFollowing, func(_, following int) int { return following })
}

// CanPostOnWall tells whether authorID may write on ownerID's wall: the
// owner's privacy decides, and "following" depends on the owner's follows,
// which is why it lives here. user.ErrNotFound for an unknown owner.
func (s *FollowService) CanPostOnWall(ctx context.Context, authorID, ownerID uuid.UUID) (bool, error) {
	owner, err := s.userRepo.GetByID(ctx, ownerID)
	if err != nil {
		return false, errs.Wrap("s.userRepo.GetByID", err)
	}

	follows, err := s.followRepo.IsFollowing(ctx, ownerID, authorID)
	if err != nil {
		return false, errs.Wrap("s.followRepo.IsFollowing", err)
	}

	return owner.CanPostOnWall(authorID, follows), nil
}

// FollowingIDs are the users whose posts make up userID's news feed.
func (s *FollowService) FollowingIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.listRepo.FollowingIDs(ctx, userID, MaxFeedSources)
	if err != nil {
		return nil, errs.Wrap("s.listRepo.FollowingIDs", err)
	}

	return ids, nil
}

type listFunc func(ctx context.Context, userID, before uuid.UUID, limit int) ([]Followed, error)

func (s *FollowService) page(
	ctx context.Context,
	userID, cursor uuid.UUID,
	size int,
	list listFunc,
	total func(followers, following int) int,
) (FollowPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra row tells whether there is a next page.
	found, err := list(ctx, userID, cursor, size+1)
	if err != nil {
		return FollowPage{}, errs.Wrap("list", err)
	}

	followers, following, err := s.followRepo.Counts(ctx, userID)
	if err != nil {
		return FollowPage{}, errs.Wrap("s.followRepo.Counts", err)
	}

	page := FollowPage{Total: total(followers, following)}

	if len(found) > size {
		found = found[:size]
		page.NextCursor = found[size-1].FollowID
	}

	page.Users = make([]user.PublicProfile, 0, len(found))
	for _, f := range found {
		page.Users = append(page.Users, f.User.PublicProfile())
	}

	return page, nil
}
