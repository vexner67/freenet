// Package roomclient closes the rooms of a deleted club through its own
// outbox stream, so a room outage cannot hold up posts or notifications.
package roomclient

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/outbox"
	roomv1 "github.com/vexner67/freenet/room/api/room/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

const Stream = "room"

type Recorder interface {
	Add(context.Context, string, []byte) error
}
type Readers struct {
	service.Readers
	recorder Recorder
}

func NewReaders(readers service.Readers, recorder Recorder) *Readers {
	return &Readers{Readers: readers, recorder: recorder}
}
func (r *Readers) Deleted(ctx context.Context, id uuid.UUID) error {
	if err := r.Readers.Deleted(ctx, id); err != nil {
		return errs.Wrap("r.Readers.Deleted", err)
	}
	return errs.Wrap("r.recorder.Add", r.recorder.Add(ctx, "purge_club", id[:]))
}
func Deliver(client roomv1.RoomServiceClient, timeout time.Duration) outbox.Deliver {
	return func(ctx context.Context, message outbox.Message) error {
		id, err := uuid.FromBytes(message.Payload)
		if err != nil {
			return outbox.Permanent(errs.Wrap("uuid.FromBytes", err))
		}
		if message.Kind != "purge_club" {
			return outbox.Permanent(fmt.Errorf("unknown room command %q", message.Kind))
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		_, err = client.PurgeClub(ctx, &roomv1.PurgeClubRequest{ClubId: id.String()})
		switch status.Code(err) {
		case codes.InvalidArgument, codes.NotFound:
			return outbox.Permanent(err)
		}
		return errs.Wrap("client.PurgeClub", err)
	}
}
