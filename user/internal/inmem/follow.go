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
	users   *UserRepository
}

func NewFollowRepository(users *UserRepository) *FollowRepository {
	r := &FollowRepository{
		follows: make(map[edge]uuid.UUID),
		users:   users,
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
		}
	}
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

func (r *FollowRepository) ListFollowers(ctx context.Context, userID, before uuid.UUID, limit int) ([]service.Followed, error) {
	return r.list(ctx, before, limit, func(e edge) (uuid.UUID, bool) { return e.follower, e.followee == userID }, nil)
}

func (r *FollowRepository) ListFollowing(ctx context.Context, userID, before uuid.UUID, limit int) ([]service.Followed, error) {
	return r.list(ctx, before, limit, following(userID), nil)
}

func (r *FollowRepository) ListOnlineFollowing(
	ctx context.Context,
	userID uuid.UUID,
	since time.Time,
	before uuid.UUID,
	limit int,
) ([]service.Followed, error) {
	return r.list(ctx, before, limit, following(userID), seenAfter(since))
}

func (r *FollowRepository) CountOnlineFollowing(ctx context.Context, userID uuid.UUID, since time.Time) (int, error) {
	found, err := r.list(ctx, uuid.Nil, math.MaxInt, following(userID), seenAfter(since))
	if err != nil {
		return 0, err
	}

	return len(found), nil
}

func (r *FollowRepository) ListCommonFollowing(ctx context.Context, userID, viewerID, before uuid.UUID, limit int) ([]service.Followed, error) {
	return r.list(ctx, before, limit, r.commonFollowing(userID, viewerID), nil)
}

func (r *FollowRepository) CountCommonFollowing(ctx context.Context, userID, viewerID uuid.UUID) (int, error) {
	found, err := r.list(ctx, uuid.Nil, math.MaxInt, r.commonFollowing(userID, viewerID), nil)
	if err != nil {
		return 0, err
	}

	return len(found), nil
}

// commonFollowing matches userID's follows of people viewerID follows
// too; list calls it holding the lock.
func (r *FollowRepository) commonFollowing(userID, viewerID uuid.UUID) func(edge) (uuid.UUID, bool) {
	return func(e edge) (uuid.UUID, bool) {
		_, viewerFollows := r.follows[edge{follower: viewerID, followee: e.followee}]
		return e.followee, e.follower == userID && viewerFollows
	}
}

func following(userID uuid.UUID) func(edge) (uuid.UUID, bool) {
	return func(e edge) (uuid.UUID, bool) { return e.followee, e.follower == userID }
}

// seenAfter keeps the people who visited after since, but deleted pages.
func seenAfter(since time.Time) func(*user.User) bool {
	return func(u *user.User) bool { return u.LastSeen().After(since) && !u.IsDeactivated() }
}

func (r *FollowRepository) FollowingIDs(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error) {
	found, err := r.list(ctx, uuid.Nil, limit, following(userID), nil)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, len(found))
	for i, f := range found {
		ids[i] = f.User.ID()
	}

	return ids, nil
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
