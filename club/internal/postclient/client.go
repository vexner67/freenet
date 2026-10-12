// Package postclient tells the post service who is in which club through
// the outbox: the news of post show a member the wall of the club. Outbox
// records a membership with the change that made it, Deliver hands it
// over afterwards. It is the only place in club that knows the post proto
// contract.
package postclient

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/outbox"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Stream is the outbox stream of the post service.
const Stream = "post"

// Outbox kinds: the payload is the club's ID and then the member's.
const (
	JoinKind  = "join_club"
	LeaveKind = "leave_club"
)

// Kinds about a club closed and opened ("Тип группы"): the payload is the
// club's ID.
const (
	CloseKind = "close_wall"
	OpenKind  = "open_wall"
)

// Recorder adds a message to the outbox in the transaction of ctx.
type Recorder interface {
	Add(ctx context.Context, kind string, payload []byte) error
}

// Outbox is the service.Readers: it records the memberships to tell.
type Outbox struct {
	recorder Recorder
}

func NewOutbox(recorder Recorder) *Outbox {
	return &Outbox{
		recorder: recorder,
	}
}

func (o *Outbox) Joined(ctx context.Context, clubID, userID uuid.UUID) error {
	return errs.Wrap("o.recorder.Add", o.recorder.Add(ctx, JoinKind, pair(clubID, userID)))
}

func (o *Outbox) Left(ctx context.Context, clubID, userID uuid.UUID) error {
	return errs.Wrap("o.recorder.Add", o.recorder.Add(ctx, LeaveKind, pair(clubID, userID)))
}

func (o *Outbox) Closed(ctx context.Context, clubID uuid.UUID) error {
	return errs.Wrap("o.recorder.Add", o.recorder.Add(ctx, CloseKind, clubID[:]))
}

func (o *Outbox) Opened(ctx context.Context, clubID uuid.UUID) error {
	return errs.Wrap("o.recorder.Add", o.recorder.Add(ctx, OpenKind, clubID[:]))
}

func pair(clubID, userID uuid.UUID) []byte {
	return append(clubID[:], userID[:]...)
}

// Nop is the service.Readers when no post service is configured.
type Nop struct{}

func (Nop) Joined(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (Nop) Left(context.Context, uuid.UUID, uuid.UUID) error   { return nil }
func (Nop) Closed(context.Context, uuid.UUID) error            { return nil }
func (Nop) Opened(context.Context, uuid.UUID) error            { return nil }

// Deliver makes the recorded calls to the post service.
func Deliver(api postv1.PostServiceClient, timeout time.Duration) outbox.Deliver {
	return func(ctx context.Context, m outbox.Message) error {
		want := 2 * len(uuid.UUID{})

		switch m.Kind {
		case JoinKind, LeaveKind:
		case CloseKind, OpenKind:
			want = len(uuid.UUID{})
		default:
			return outbox.Permanent(fmt.Errorf("unknown kind %q", m.Kind))
		}

		if len(m.Payload) != want {
			return outbox.Permanent(fmt.Errorf("%s of %d bytes", m.Kind, len(m.Payload)))
		}

		clubID := uuid.UUID(m.Payload[:16]).String()

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var err error

		switch m.Kind {
		case JoinKind:
			_, err = api.FollowSource(ctx, &postv1.FollowSourceRequest{ReaderId: uuid.UUID(m.Payload[16:]).String(), SourceId: clubID, Club: true})
		case LeaveKind:
			_, err = api.UnfollowSource(ctx, &postv1.UnfollowSourceRequest{ReaderId: uuid.UUID(m.Payload[16:]).String(), SourceId: clubID})
		case CloseKind:
			_, err = api.CloseWall(ctx, &postv1.CloseWallRequest{WallId: clubID, Club: true})
		default:
			_, err = api.OpenWall(ctx, &postv1.OpenWallRequest{WallId: clubID})
		}

		// A request post rejects stays rejected; anything else is tried
		// again — post down in the first place, and a post that does not
		// know the call yet, an older version still running while the
		// site is rolled out.
		switch status.Code(err) {
		case codes.OK:
			return nil
		case codes.InvalidArgument, codes.NotFound:
			return outbox.Permanent(err)
		default:
			return err
		}
	}
}
