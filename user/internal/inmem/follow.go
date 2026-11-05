package inmem

import (
	"bytes"
	"context"
	"errors"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// edge is a directed pair: a follow (follower → followee) or, in
// BlockRepository, a block (blocker → blocked).
type edge struct {
	follower, followee uuid.UUID
}

// FollowRepository keeps follows in memory and reads users from the
// in-memory UserRepository, as PostgreSQL joins the two tables.
type FollowRepository struct {
	mu      sync.RWMutex
	follows map[edge]uuid.UUID // follow ID by edge
	// declined are the follows left among the followers.
	declined map[edge]bool
	users    *UserRepository
}

func NewFollowRepository(users *UserRepository) *FollowRepository {
	r := &FollowRepository{
		follows:  make(map[edge]uuid.UUID),
		declined: make(map[edge]bool),
		users:    users,
	}
	users.onDelete(r.forget)

	return r
}

// forget drops the follows of a deleted user, both ways.
func (r *FollowRepository) forget(userID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for e := range r.follows {
		if e.follower == userID || e.followee == userID {
			delete(r.follows, e)
			delete(r.declined, e)
		}
	}
}

func (r *FollowRepository) Readers(ctx context.Context, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	return above(r.users.ids(), after, limit), nil
}

func (r *FollowRepository) Followees(ctx context.Context, readerID, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var ids []uuid.UUID
	for e := range r.follows {
		if e.follower == readerID {
			ids = append(ids, e.followee)
		}
	}

	return above(ids, after, limit), nil
}

// above are up to limit of ids above after, ascending.
func above(ids []uuid.UUID, after uuid.UUID, limit int) []uuid.UUID {
	ids = slices.DeleteFunc(ids, func(id uuid.UUID) bool {
		return bytes.Compare(id[:], after[:]) <= 0
	})

	slices.SortFunc(ids, func(a, b uuid.UUID) int {
		return bytes.Compare(a[:], b[:])
	})

	return ids[:min(limit, len(ids))]
}

// LockReader: there are no transactions in memory to hold a lock.
func (r *FollowRepository) LockReader(ctx context.Context, _ uuid.UUID) error {
	return errs.Wrap("ctx.Err", ctx.Err())
}

func (r *FollowRepository) Follow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	e := edge{followerID, followeeID}
	if _, ok := r.follows[e]; !ok {
		r.follows[e] = id
	}

	return nil
}

func (r *FollowRepository) Unfollow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.follows, edge{followerID, followeeID})
	delete(r.declined, edge{followerID, followeeID})

	return nil
}

func (r *FollowRepository) IsFollowing(ctx context.Context, followerID, followeeID uuid.UUID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.follows[edge{followerID, followeeID}]

	return ok, nil
}

func (r *FollowRepository) Counts(ctx context.Context, userID uuid.UUID) (followers, following int, err error) {
	if err = ctx.Err(); err != nil {
		return 0, 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for e := range r.follows {
		if e.followee == userID {
			followers++
		}
		if e.follower == userID {
			following++
		}
	}

	return followers, following, nil
}

func (r *FollowRepository) ListFriends(ctx context.Context, q service.FriendQuery, before uuid.UUID, limit int) ([]service.Followed, error) {
	match, keep, err := r.friendMatch(q)
	if err != nil {
		return nil, err
	}

	return r.list(ctx, before, limit, match, keep)
}

func (r *FollowRepository) CountFriends(ctx context.Context, q service.FriendQuery) (int, error) {
	found, err := r.ListFriends(ctx, q, uuid.Nil, math.MaxInt)
	if err != nil {
		return 0, err
	}

	return len(found), nil
}

func (r *FollowRepository) Decline(ctx context.Context, followerID, followeeID uuid.UUID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	e := edge{followerID, followeeID}
	if _, ok := r.follows[e]; !ok {
		return false, nil
	}

	r.declined[e] = true

	return true, nil
}

// friendMatch picks the follows of a section, as the queries of
// PostgreSQL do; list calls the match holding the lock.
func (r *FollowRepository) friendMatch(q service.FriendQuery) (func(edge) (uuid.UUID, bool), func(*user.User) bool, error) {
	has := func(follower, followee uuid.UUID) bool {
		_, ok := r.follows[edge{follower, followee}]
		return ok
	}
	friend := func(e edge) (uuid.UUID, bool) {
		return e.followee, e.follower == q.UserID && has(e.followee, e.follower)
	}

	switch q.Section {
	case user.FriendsAll:
		return friend, nil, nil
	case user.FriendsOnline:
		return friend, seenAfter(q.Since), nil
	case user.FriendsCommon:
		return func(e edge) (uuid.UUID, bool) {
			other, ok := friend(e)
			return other, ok && has(q.ViewerID, other) && has(other, q.ViewerID)
		}, nil, nil
	case user.FriendRequests:
		return func(e edge) (uuid.UUID, bool) {
			return e.follower, e.followee == q.UserID && !has(q.UserID, e.follower) && !r.declined[e]
		}, nil, nil
	case user.FriendFollowers:
		return func(e edge) (uuid.UUID, bool) {
			return e.follower, e.followee == q.UserID && !has(q.UserID, e.follower)
		}, nil, nil
	case user.FriendsOutgoing:
		return func(e edge) (uuid.UUID, bool) {
			return e.followee, e.follower == q.UserID && !has(e.followee, q.UserID)
		}, nil, nil
	default:
		return nil, nil, user.ErrInvalidFriendSection
	}
}

// seenAfter keeps the people who visited after since, but deleted pages.
func seenAfter(since time.Time) func(*user.User) bool {
	return func(u *user.User) bool { return u.LastSeen().After(since) && !u.IsDeactivated() }
}

// list returns the other side of matching follows, newest follow first;
// keep, if set, picks the users.
func (r *FollowRepository) list(
	ctx context.Context,
	before uuid.UUID,
	limit int,
	match func(edge) (other uuid.UUID, ok bool),
	keep func(*user.User) bool,
) ([]service.Followed, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()

	type hit struct {
		followID, other uuid.UUID
	}

	var hits []hit
	for e, id := range r.follows {
		other, ok := match(e)
		if ok && (before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0) {
			hits = append(hits, hit{id, other})
		}
	}

	r.mu.RUnlock()

	slices.SortFunc(hits, func(a, b hit) int { return bytes.Compare(b.followID[:], a.followID[:]) })

	found := make([]service.Followed, 0, min(len(hits), limit))
	for _, h := range hits {
		if len(found) == limit {
			break
		}

		u, err := r.users.GetByID(ctx, h.other)
		if err != nil && !errors.Is(err, user.ErrNotFound) {
			return nil, errs.Wrap("r.users.GetByID", err)
		}
		if u != nil && (keep == nil || keep(u)) {
			found = append(found, service.Followed{FollowID: h.followID, User: u})
		}
	}

	return found, nil
}
