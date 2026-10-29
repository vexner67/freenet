// Package purgeclient tells the services that keep data of users to erase
// a user whose page was deleted for good, through the outbox: Outbox
// records it with the deletion, the deliverers call each service's
// PurgeUser afterwards. It is the only place in user that knows their
// proto contracts.
package purgeclient

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/outbox"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Kind is the outbox kind of an erasure; the payload is the user's ID.
const Kind = "purge_user"

// Kinds of the post stream about a page deleted and restored: its likes
// and poll votes are set aside and put back. The payload is the user's ID.
const (
	HoldKind    = "hold_reactions"
	ReleaseKind = "release_reactions"
)

// Kinds of the post stream about a follow and its end: post keeps whom
// everyone reads in the news. The payload is the follower's ID and then
// the followee's.
const (
	FollowKind   = "follow_source"
	UnfollowKind = "unfollow_source"
)

// SyncKind is the kind of the post stream about a stretch of all a
// reader follows, told now and then so that the copy of post matches.
const SyncKind = "sync_follows"

// Kinds of the post stream about a page closed and opened ("Тип
// профиля"): post stops showing its wall to strangers. The payload is the
// user's ID.
const (
	CloseWallKind = "close_wall"
	OpenWallKind  = "open_wall"
)

// Streams of the services that keep data of users, besides notification,
// whose stream it shares with the notifications, so that a follow never
// arrives after its follower is erased.
const (
	PostStream    = "post"
	MediaStream   = "media"
	MessageStream = "message"
	ClubStream    = "club"
)

// Recorder adds a message to an outbox stream in the transaction of ctx.
type Recorder interface {
	Add(ctx context.Context, kind string, payload []byte) error
}

// Outbox is the service.Eraser: it records the erasure in the stream of
// every service that keeps data of users.
type Outbox struct {
	streams []Recorder
}

func NewOutbox(streams ...Recorder) *Outbox {
	return &Outbox{
		streams: streams,
	}
}

func (o *Outbox) UserErased(ctx context.Context, userID uuid.UUID) error {
	for _, s := range o.streams {
		if err := s.Add(ctx, Kind, userID[:]); err != nil {
			return errs.Wrap("s.Add", err)
		}
	}

	return nil
}

// Reactions is the service.Reactions: it records a deleted and a restored
// page in the post stream, the one that also carries the erasure, so the
// three arrive in the order they happened.
type Reactions struct {
	post Recorder
}

func NewReactions(post Recorder) *Reactions {
	return &Reactions{
		post: post,
	}
}

func (r *Reactions) UserDeactivated(ctx context.Context, userID uuid.UUID) error {
	return errs.Wrap("r.post.Add", r.post.Add(ctx, HoldKind, userID[:]))
}

func (r *Reactions) UserReactivated(ctx context.Context, userID uuid.UUID) error {
	return errs.Wrap("r.post.Add", r.post.Add(ctx, ReleaseKind, userID[:]))
}

// Walls is the service.Walls: it records a closed and an opened page in
// the post stream, in order with the follows that decide who is let in.
type Walls struct {
	post Recorder
}

func NewWalls(post Recorder) *Walls {
	return &Walls{
		post: post,
	}
}

func (w *Walls) PageClosed(ctx context.Context, userID uuid.UUID) error {
	return errs.Wrap("w.post.Add", w.post.Add(ctx, CloseWallKind, userID[:]))
}

func (w *Walls) PageOpened(ctx context.Context, userID uuid.UUID) error {
	return errs.Wrap("w.post.Add", w.post.Add(ctx, OpenWallKind, userID[:]))
}

// NoReactions is the service.Reactions when no post service is configured.
type NoReactions struct{}

func (NoReactions) UserDeactivated(context.Context, uuid.UUID) error { return nil }
func (NoReactions) UserReactivated(context.Context, uuid.UUID) error { return nil }

// Locker keeps everyone else from telling of the follows of a reader
// until the transaction of ctx ends.
type Locker interface {
	LockReader(ctx context.Context, readerID uuid.UUID) error
}

// Follows is a service.Notifier: it records follows and their ends in the
// post stream, so that the news of post follow them, in order, and an
// erased user's follows never arrive after the erasure. It is the
// service.Sources too: the stretches of all a reader follows, told now
// and then, go the same way. Each is recorded under the lock of the
// reader, so a follow and a stretch are recorded in the order they
// became true.
type Follows struct {
	post Recorder
	lock Locker
}

func NewFollows(post Recorder, lock Locker) *Follows {
	return &Follows{
		post: post,
		lock: lock,
	}
}

func (f *Follows) Followed(ctx context.Context, followerID, followeeID uuid.UUID) error {
	return f.add(ctx, followerID, FollowKind, pair(followerID, followeeID))
}

// Invited is a follow to post: the news do not tell invitations apart.
func (f *Follows) Invited(ctx context.Context, inviteeID, inviterID uuid.UUID) error {
	return f.Followed(ctx, inviteeID, inviterID)
}

func (f *Follows) Unfollowed(ctx context.Context, followerID, followeeID uuid.UUID) error {
	return f.add(ctx, followerID, UnfollowKind, pair(followerID, followeeID))
}

func (f *Follows) Following(ctx context.Context, readerID, after uuid.UUID, ids []uuid.UUID, last bool) error {
	return f.add(ctx, readerID, SyncKind, stretch(readerID, after, ids, last))
}

func (f *Follows) add(ctx context.Context, readerID uuid.UUID, kind string, payload []byte) error {
	if err := f.lock.LockReader(ctx, readerID); err != nil {
		return errs.Wrap("f.lock.LockReader", err)
	}

	return errs.Wrap("f.post.Add", f.post.Add(ctx, kind, payload))
}

// stretch is the payload of SyncKind: the reader, the ID the stretch
// starts above, whether it is the last one, and the IDs.
func stretch(readerID, after uuid.UUID, ids []uuid.UUID, last bool) []byte {
	payload := make([]byte, 0, stretchHead+len(ids)*len(uuid.UUID{}))
	payload = append(payload, readerID[:]...)
	payload = append(payload, after[:]...)

	if last {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}

	for _, id := range ids {
		payload = append(payload, id[:]...)
	}

	return payload
}

// stretchHead is what a stretch holds before its IDs.
const stretchHead = 2*len(uuid.UUID{}) + 1

// syncRequest reads the payload of SyncKind.
func syncRequest(payload []byte) (*postv1.SyncSourcesRequest, bool) {
	const size = len(uuid.UUID{})

	if len(payload) < stretchHead || (len(payload)-stretchHead)%size != 0 {
		return nil, false
	}

	req := &postv1.SyncSourcesRequest{
		OwnerId: uuid.UUID(payload[:size]).String(),
		AfterId: uuid.UUID(payload[size : 2*size]).String(),
		Last:    payload[2*size] == 1,
	}

	for rest := payload[stretchHead:]; len(rest) > 0; rest = rest[size:] {
		req.Ids = append(req.Ids, uuid.UUID(rest[:size]).String())
	}

	return req, true
}

// pair is the payload of a follow: who reads and whom.
func pair(followerID, followeeID uuid.UUID) []byte {
	return append(followerID[:], followeeID[:]...)
}

// purgeFunc calls a service's RPC about the user.
type purgeFunc func(ctx context.Context, userID string) error

func Post(api postv1.PostServiceClient, timeout time.Duration) outbox.Deliver {
	users := deliverKinds(timeout, map[string]purgeFunc{
		Kind: func(ctx context.Context, id string) error {
			_, err := api.PurgeUser(ctx, &postv1.PurgeUserRequest{UserId: id})
			return err
		},
		HoldKind: func(ctx context.Context, id string) error {
			_, err := api.HoldUserReactions(ctx, &postv1.HoldUserReactionsRequest{UserId: id})
			return err
		},
		ReleaseKind: func(ctx context.Context, id string) error {
			_, err := api.ReleaseUserReactions(ctx, &postv1.ReleaseUserReactionsRequest{UserId: id})
			return err
		},
		CloseWallKind: func(ctx context.Context, id string) error {
			_, err := api.CloseWall(ctx, &postv1.CloseWallRequest{WallId: id})
			return err
		},
		OpenWallKind: func(ctx context.Context, id string) error {
			_, err := api.OpenWall(ctx, &postv1.OpenWallRequest{WallId: id})
			return err
		},
	})

	return func(ctx context.Context, m outbox.Message) error {
		if m.Kind == SyncKind {
			req, ok := syncRequest(m.Payload)
			if !ok {
				return outbox.Permanent(fmt.Errorf("a stretch of %d bytes", len(m.Payload)))
			}

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			_, err := api.SyncSources(ctx, req)

			return outcome(err)
		}

		if m.Kind != FollowKind && m.Kind != UnfollowKind {
			return users(ctx, m)
		}

		if len(m.Payload) != 2*len(uuid.UUID{}) {
			return outbox.Permanent(fmt.Errorf("a follow of %d bytes", len(m.Payload)))
		}

		follower, followee := uuid.UUID(m.Payload[:16]).String(), uuid.UUID(m.Payload[16:]).String()

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var err error
		if m.Kind == FollowKind {
			_, err = api.FollowSource(ctx, &postv1.FollowSourceRequest{ReaderId: follower, SourceId: followee})
		} else {
			_, err = api.UnfollowSource(ctx, &postv1.UnfollowSourceRequest{ReaderId: follower, SourceId: followee})
		}

		return outcome(err)
	}
}

func Media(api mediav1.MediaServiceClient, timeout time.Duration) outbox.Deliver {
	return deliver(timeout, func(ctx context.Context, id string) error {
		_, err := api.PurgeUser(ctx, &mediav1.PurgeUserRequest{UserId: id})
		return err
	})
}

func Message(api messagev1.MessageServiceClient, timeout time.Duration) outbox.Deliver {
	return deliver(timeout, func(ctx context.Context, id string) error {
		_, err := api.PurgeUser(ctx, &messagev1.PurgeUserRequest{UserId: id})
		return err
	})
}

func Club(api clubv1.ClubServiceClient, timeout time.Duration) outbox.Deliver {
	return deliver(timeout, func(ctx context.Context, id string) error {
		_, err := api.PurgeUser(ctx, &clubv1.PurgeUserRequest{UserId: id})
		return err
	})
}

// Notification delivers the erasures of the notification stream and hands
// the rest, the notifications, to others.
func Notification(api notificationv1.NotificationServiceClient, timeout time.Duration, others outbox.Deliver) outbox.Deliver {
	purge := deliver(timeout, func(ctx context.Context, id string) error {
		_, err := api.PurgeUser(ctx, &notificationv1.PurgeUserRequest{UserId: id})
		return err
	})

	return func(ctx context.Context, m outbox.Message) error {
		if m.Kind == Kind {
			return purge(ctx, m)
		}

		return others(ctx, m)
	}
}

func deliver(timeout time.Duration, purge purgeFunc) outbox.Deliver {
	return deliverKinds(timeout, map[string]purgeFunc{Kind: purge})
}

// deliverKinds delivers each kind of a stream by its call.
func deliverKinds(timeout time.Duration, calls map[string]purgeFunc) outbox.Deliver {
	return func(ctx context.Context, m outbox.Message) error {
		purge, ok := calls[m.Kind]
		if !ok {
			return outbox.Permanent(fmt.Errorf("unknown kind %q", m.Kind))
		}

		userID, err := uuid.FromBytes(m.Payload)
		if err != nil {
			return outbox.Permanent(errs.Wrap("uuid.FromBytes", err))
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		return outcome(purge(ctx, userID.String()))
	}
}

// outcome tells the relay what to do with the answer of a service: a
// request it rejects stays rejected; anything else is tried again — a
// service down in the first place, and one that does not know the call
// yet, an older version still running while the site is rolled out.
func outcome(err error) error {
	switch status.Code(err) {
	case codes.OK:
		return nil
	case codes.InvalidArgument, codes.NotFound:
		return outbox.Permanent(err)
	default:
		return err
	}
}
