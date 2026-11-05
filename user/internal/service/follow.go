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

	// MaxFeedSources bounds how many blocked authors a news feed leaves out.
	MaxFeedSources = 1000
)

type FollowRepository interface {
	// Follow and Unfollow are idempotent.
	Follow(ctx context.Context, followerID, followeeID uuid.UUID) error
	Unfollow(ctx context.Context, followerID, followeeID uuid.UUID) error
	IsFollowing(ctx context.Context, followerID, followeeID uuid.UUID) (bool, error)
	Counts(ctx context.Context, userID uuid.UUID) (followers, following int, err error)
}

// InviteRepository remembers who invited a page to the site.
type InviteRepository interface {
	// SetInviter names inviterID as who invited userID; user.ErrAlreadyInvited
	// when someone is named already.
	SetInviter(ctx context.Context, userID, inviterID uuid.UUID) error
}

// FriendListRepository lists a section of a user's friends, newest
// follow first. Cursors are follow IDs (UUIDv7).
type FriendListRepository interface {
	ListFriends(ctx context.Context, q FriendQuery, before uuid.UUID, limit int) ([]Followed, error)
	CountFriends(ctx context.Context, q FriendQuery) (int, error)
	// Decline leaves the follow of followerID among followeeID's followers,
	// out of the requests; false when there is no such follow.
	Decline(ctx context.Context, followerID, followeeID uuid.UUID) (bool, error)
}

// FriendQuery is which friends of UserID to list: ViewerID is whose
// friends the common ones are, Since when the online ones visited last
// (deleted pages are never online).
type FriendQuery struct {
	UserID   uuid.UUID
	ViewerID uuid.UUID
	Section  user.FriendSection
	Since    time.Time
}

// Followed is a user in a follow list with the ID of that follow.
type Followed struct {
	FollowID uuid.UUID
	User     *user.User
}

type FollowStats struct {
	Followers int
	Following int
	// Friends follow the user, who follows them.
	Friends int
	// Requests are the user's friend requests, told the user only.
	Requests int
	// IsFollowing: the viewer follows the user; false for anonymous viewers.
	IsFollowing bool
	// FollowsViewer: the user follows the viewer, who is a friend when
	// following back and has the user's request otherwise.
	FollowsViewer bool
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
	listRepo   FriendListRepository
	blockRepo  BlockChecker
	inviteRepo InviteRepository
	tx         TxManager
	notifier   Notifier
	now        func() time.Time
}

// FollowDeps are what FollowService works with.
type FollowDeps struct {
	Users    UserRepository
	Follows  FollowRepository
	Lists    FriendListRepository
	Blocks   BlockChecker
	Invites  InviteRepository
	Tx       TxManager
	Notifier Notifier
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

func NewFollowService(d FollowDeps) *FollowService {
	if d.Now == nil {
		d.Now = time.Now
	}

	return &FollowService{
		userRepo:   d.Users,
		followRepo: d.Follows,
		listRepo:   d.Lists,
		blockRepo:  d.Blocks,
		inviteRepo: d.Invites,
		tx:         d.Tx,
		notifier:   d.Notifier,
		now:        d.Now,
	}
}

func (s *FollowService) Follow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	if err := s.checkFollow(ctx, followerID, followeeID); err != nil {
		return err
	}

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.followRepo.Follow(ctx, followerID, followeeID); err != nil {
			return errs.Wrap("s.followRepo.Follow", err)
		}

		// Following back accepts the friend request ("Принять заявку").
		accepted, err := s.followRepo.IsFollowing(ctx, followeeID, followerID)
		if err != nil {
			return errs.Wrap("s.followRepo.IsFollowing", err)
		}

		// Following twice notifies once: the notification service keeps
		// one notification per follow.
		if accepted {
			return errs.Wrap("s.notifier.Befriended", s.notifier.Befriended(ctx, followerID, followeeID))
		}

		return errs.Wrap("s.notifier.Followed", s.notifier.Followed(ctx, followerID, followeeID))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// AcceptInvite is a new page following the one whose invitation link it
// signed up by, openvk's "Пригласить": once, while the page is new, and
// as a follow would be; the inviter hears of the invitation, not of a
// follow.
func (s *FollowService) AcceptInvite(ctx context.Context, inviteeID, inviterID uuid.UUID) error {
	invitee, err := s.userRepo.GetByID(ctx, inviteeID)
	if err != nil {
		return errs.Wrap("s.userRepo.GetByID", err)
	}

	if err := invitee.CheckInvitedAt(s.now().UTC()); err != nil {
		return errs.Wrap("invitee.CheckInvitedAt", err)
	}

	if err := s.checkFollow(ctx, inviteeID, inviterID); err != nil {
		return err
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.inviteRepo.SetInviter(ctx, inviteeID, inviterID); err != nil {
			return errs.Wrap("s.inviteRepo.SetInviter", err)
		}

		if err := s.followRepo.Follow(ctx, inviteeID, inviterID); err != nil {
			return errs.Wrap("s.followRepo.Follow", err)
		}

		return errs.Wrap("s.notifier.Invited", s.notifier.Invited(ctx, inviteeID, inviterID))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// checkFollow tells whether followerID may follow followeeID.
func (s *FollowService) checkFollow(ctx context.Context, followerID, followeeID uuid.UUID) error {
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

// Decline leaves the friend request of followerID among ownerID's
// followers ("Оставить в подписчиках"): the follow stays.
func (s *FollowService) Decline(ctx context.Context, ownerID, followerID uuid.UUID) error {
	declined, err := s.listRepo.Decline(ctx, followerID, ownerID)
	if err != nil {
		return errs.Wrap("s.listRepo.Decline", err)
	}

	if !declined {
		return user.ErrNoFriendRequest
	}

	return nil
}

// Stats counts the user's follows and friends, and the requests for the
// user only; viewerID is uuid.Nil for an anonymous viewer.
func (s *FollowService) Stats(ctx context.Context, userID, viewerID uuid.UUID) (FollowStats, error) {
	followers, following, err := s.followRepo.Counts(ctx, userID)
	if err != nil {
		return FollowStats{}, errs.Wrap("s.followRepo.Counts", err)
	}

	stats := FollowStats{Followers: followers, Following: following}

	if stats.Friends, err = s.listRepo.CountFriends(ctx, FriendQuery{UserID: userID, Section: user.FriendsAll}); err != nil {
		return FollowStats{}, errs.Wrap("s.listRepo.CountFriends", err)
	}

	if viewerID == userID {
		if stats.Requests, err = s.listRepo.CountFriends(ctx, FriendQuery{UserID: userID, Section: user.FriendRequests}); err != nil {
			return FollowStats{}, errs.Wrap("s.listRepo.CountFriends", err)
		}
	}

	if viewerID != uuid.Nil && viewerID != userID {
		if stats.IsFollowing, err = s.followRepo.IsFollowing(ctx, viewerID, userID); err != nil {
			return FollowStats{}, errs.Wrap("s.followRepo.IsFollowing", err)
		}

		if stats.FollowsViewer, err = s.followRepo.IsFollowing(ctx, userID, viewerID); err != nil {
			return FollowStats{}, errs.Wrap("s.followRepo.IsFollowing", err)
		}
	}

	return stats, nil
}

// ListFriends is a section of openvk's "Друзья" of userID as viewerID
// (uuid.Nil: anonymous) sees it: the requests, the followers and the
// outgoing requests are the owner's only, and the common friends need a
// viewer.
func (s *FollowService) ListFriends(
	ctx context.Context,
	userID, viewerID uuid.UUID,
	section user.FriendSection,
	cursor uuid.UUID,
	size int,
) (FollowPage, error) {
	if section.OwnOnly() && viewerID != userID {
		return FollowPage{}, user.ErrNotYourRequests
	}

	q := FriendQuery{UserID: userID, ViewerID: viewerID, Section: section}
	if section == user.FriendsOnline {
		q.Since = s.now().UTC().Add(-user.OnlineFor)
	}

	if section == user.FriendsCommon && (viewerID == uuid.Nil || viewerID == userID) {
		return FollowPage{}, nil
	}

	list := func(ctx context.Context, _, before uuid.UUID, limit int) ([]Followed, error) {
		return s.listRepo.ListFriends(ctx, q, before, limit)
	}

	return s.page(ctx, userID, cursor, size, list, func(ctx context.Context) (int, error) {
		n, err := s.listRepo.CountFriends(ctx, q)
		return n, errs.Wrap("s.listRepo.CountFriends", err)
	})
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
		page.Users = append(page.Users, f.User.ListedProfile(s.now()))
	}

	return page, nil
}
