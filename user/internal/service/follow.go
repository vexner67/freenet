package service

import (
	"context"
	"time"

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
	// ListOnlineFollowing and CountOnlineFollowing are whom userID follows
	// who visited after since, but deleted pages.
	ListOnlineFollowing(ctx context.Context, userID uuid.UUID, since time.Time, before uuid.UUID, limit int) ([]Followed, error)
	CountOnlineFollowing(ctx context.Context, userID uuid.UUID, since time.Time) (int, error)
	// ListCommonFollowing and CountCommonFollowing are whom both userID and
	// viewerID follow, by userID's follows.
	ListCommonFollowing(ctx context.Context, userID, viewerID, before uuid.UUID, limit int) ([]Followed, error)
	CountCommonFollowing(ctx context.Context, userID, viewerID uuid.UUID) (int, error)
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
	blockRepo  BlockChecker
	tx         TxManager
	notifier   Notifier
	now        func() time.Time
}

// FollowDeps are what FollowService works with.
type FollowDeps struct {
	Users    UserRepository
	Follows  FollowRepository
	Lists    FollowListRepository
	Blocks   BlockChecker
	Tx       TxManager
	Notifier Notifier
}

func NewFollowService(d FollowDeps) *FollowService {
	return &FollowService{
		userRepo:   d.Users,
		followRepo: d.Follows,
		listRepo:   d.Lists,
		blockRepo:  d.Blocks,
		tx:         d.Tx,
		notifier:   d.Notifier,
		now:        time.Now,
	}
}

func (s *FollowService) Follow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	if followerID == followeeID {
		return user.ErrFollowSelf
	}

	followee, err := s.userRepo.GetByID(ctx, followeeID)
	if err != nil {
		return errs.Wrap("s.userRepo.GetByID", err)
	}

	if followee.IsDeactivated() {
		return user.ErrDeactivated
	}

	// A follow racing a block may slip in; the next block drops it.
	byFollower, byFollowee, err := s.blockRepo.Blocks(ctx, followerID, followeeID)
	if err != nil {
		return errs.Wrap("s.blockRepo.Blocks", err)
	}

	if byFollower || byFollowee {
		return user.ErrFollowBlocked
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.followRepo.Follow(ctx, followerID, followeeID); err != nil {
			return errs.Wrap("s.followRepo.Follow", err)
		}

		// Following twice notifies once: the notification service keeps
		// one notification per follow.
		return errs.Wrap("s.notifier.Followed", s.notifier.Followed(ctx, followerID, followeeID))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

func (s *FollowService) Unfollow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.followRepo.Unfollow(ctx, followerID, followeeID); err != nil {
			return errs.Wrap("s.followRepo.Unfollow", err)
		}

		if followerID == followeeID {
			return nil
		}

		return errs.Wrap("s.notifier.Unfollowed", s.notifier.Unfollowed(ctx, followerID, followeeID))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
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
	return s.page(ctx, userID, cursor, size, s.listRepo.ListFollowers, func(ctx context.Context) (int, error) {
		followers, _, err := s.followRepo.Counts(ctx, userID)
		return followers, errs.Wrap("s.followRepo.Counts", err)
	})
}

func (s *FollowService) ListFollowing(ctx context.Context, userID, cursor uuid.UUID, size int) (FollowPage, error) {
	return s.page(ctx, userID, cursor, size, s.listRepo.ListFollowing, func(ctx context.Context) (int, error) {
		_, following, err := s.followRepo.Counts(ctx, userID)
		return following, errs.Wrap("s.followRepo.Counts", err)
	})
}

// ListOnlineFollowing is whom userID follows who is online now, openvk's
// "Друзья онлайн".
func (s *FollowService) ListOnlineFollowing(ctx context.Context, userID, cursor uuid.UUID, size int) (FollowPage, error) {
	since := s.now().UTC().Add(-user.OnlineFor)

	list := func(ctx context.Context, userID, before uuid.UUID, limit int) ([]Followed, error) {
		return s.listRepo.ListOnlineFollowing(ctx, userID, since, before, limit)
	}

	return s.page(ctx, userID, cursor, size, list, func(ctx context.Context) (int, error) {
		online, err := s.listRepo.CountOnlineFollowing(ctx, userID, since)
		return online, errs.Wrap("s.listRepo.CountOnlineFollowing", err)
	})
}

// ListCommonFollowing is whom both userID and viewerID follow, openvk's
// "Общие друзья".
func (s *FollowService) ListCommonFollowing(ctx context.Context, userID, viewerID, cursor uuid.UUID, size int) (FollowPage, error) {
	list := func(ctx context.Context, userID, before uuid.UUID, limit int) ([]Followed, error) {
		return s.listRepo.ListCommonFollowing(ctx, userID, viewerID, before, limit)
	}

	return s.page(ctx, userID, cursor, size, list, func(ctx context.Context) (int, error) {
		common, err := s.listRepo.CountCommonFollowing(ctx, userID, viewerID)
		return common, errs.Wrap("s.listRepo.CountCommonFollowing", err)
	})
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
	total func(ctx context.Context) (int, error),
) (FollowPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra row tells whether there is a next page.
	found, err := list(ctx, userID, cursor, size+1)
	if err != nil {
		return FollowPage{}, errs.Wrap("list", err)
	}

	count, err := total(ctx)
	if err != nil {
		return FollowPage{}, errs.Wrap("total", err)
	}

	page := FollowPage{Total: count}

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
