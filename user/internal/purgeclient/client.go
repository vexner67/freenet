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

// purgeFunc calls a service's PurgeUser.
type purgeFunc func(ctx context.Context, userID string) error

func Post(api postv1.PostServiceClient, timeout time.Duration) outbox.Deliver {
	return deliver(timeout, func(ctx context.Context, id string) error {
		_, err := api.PurgeUser(ctx, &postv1.PurgeUserRequest{UserId: id})
		return err
	})
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
	return func(ctx context.Context, m outbox.Message) error {
		if m.Kind != Kind {
			return outbox.Permanent(fmt.Errorf("unknown kind %q", m.Kind))
		}

		userID, err := uuid.FromBytes(m.Payload)
		if err != nil {
			return outbox.Permanent(errs.Wrap("uuid.FromBytes", err))
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// A request the service rejects stays rejected; anything else,
		// a service down in the first place, is tried again.
		switch err = purge(ctx, userID.String()); status.Code(err) {
		case codes.OK:
			return nil
		case codes.InvalidArgument, codes.NotFound, codes.Unimplemented:
			return outbox.Permanent(err)
		default:
			return err
		}
	}
}
