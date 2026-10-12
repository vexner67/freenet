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

// NoReactions is the service.Reactions when no post service is configured.
type NoReactions struct{}

func (NoReactions) UserDeactivated(context.Context, uuid.UUID) error { return nil }
func (NoReactions) UserReactivated(context.Context, uuid.UUID) error { return nil }

// Follows is a service.Notifier: it records follows and their ends in the
// post stream, so that the news of post follow them, in order, and an
// erased user's follows never arrive after the erasure.
type Follows struct {
	post Recorder
}

func NewFollows(post Recorder) *Follows {
	return &Follows{
		post: post,
	}
}

func (f *Follows) Followed(ctx context.Context, followerID, followeeID uuid.UUID) error {
	return errs.Wrap("f.post.Add", f.post.Add(ctx, FollowKind, pair(followerID, followeeID)))
}

func (f *Follows) Unfollowed(ctx context.Context, followerID, followeeID uuid.UUID) error {
	return errs.Wrap("f.post.Add", f.post.Add(ctx, UnfollowKind, pair(followerID, followeeID)))
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
	})

	return func(ctx context.Context, m outbox.Message) error {
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
