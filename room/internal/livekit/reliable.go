package livekit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/room/internal/service"
)

const Stream = "livekit"

type Recorder interface {
	Add(context.Context, string, []byte) error
}
type command struct{ RoomID, UserID uuid.UUID }

// Reliable records removals before trying them immediately. A media outage
// must not permanently leave a kicked person's audio connected.
type Reliable struct {
	media    service.MediaServer
	recorder Recorder
	presence service.PresenceIndex
	bans     service.BanRepository
	clock    service.Clock
	log      *slog.Logger
}

func NewReliable(media service.MediaServer, recorder Recorder, presence service.PresenceIndex, bans service.BanRepository, clock service.Clock, log *slog.Logger) *Reliable {
	return &Reliable{media: media, recorder: recorder, presence: presence, bans: bans, clock: clock, log: log}
}
func (r *Reliable) Token(ctx context.Context, id, user uuid.UUID) (service.MediaToken, error) {
	return r.media.Token(ctx, id, user)
}
func (r *Reliable) Mute(ctx context.Context, id, user uuid.UUID) error {
	return r.media.Mute(ctx, id, user)
}
func (r *Reliable) Remove(ctx context.Context, id, user uuid.UUID) error {
	if err := r.record(ctx, "remove", command{RoomID: id, UserID: user}); err != nil {
		return errs.Wrap("r.record", err)
	}
	if err := r.media.Remove(ctx, id, user); err != nil {
		r.log.WarnContext(ctx, "media removal queued for retry", "error", err)
	}
	return nil
}
func (r *Reliable) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.record(ctx, "delete", command{RoomID: id}); err != nil {
		return errs.Wrap("r.record", err)
	}
	if err := r.media.Delete(ctx, id); err != nil {
		r.log.WarnContext(ctx, "media room deletion queued for retry", "error", err)
	}
	return nil
}
func (r *Reliable) record(ctx context.Context, kind string, c command) error {
	payload, err := json.Marshal(c)
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}
	return errs.Wrap("r.recorder.Add", r.recorder.Add(ctx, kind, payload))
}
func (r *Reliable) Deliver(ctx context.Context, message outbox.Message) error {
	var c command
	if err := json.Unmarshal(message.Payload, &c); err != nil {
		return outbox.Permanent(errs.Wrap("json.Unmarshal", err))
	}
	if c.RoomID == uuid.Nil {
		return outbox.Permanent(fmt.Errorf("invalid media room ID"))
	}
	switch message.Kind {
	case "delete":
		return errs.Wrap("r.media.Delete", r.media.Delete(ctx, c.RoomID))
	case "remove":
		if c.UserID == uuid.Nil {
			return outbox.Permanent(fmt.Errorf("invalid media user ID"))
		}
		until, err := r.bans.Until(ctx, c.RoomID, c.UserID)
		if err != nil {
			return errs.Wrap("r.bans.Until", err)
		}
		if !until.After(r.clock.Now()) {
			current, err := r.presence.Current(ctx, []uuid.UUID{c.UserID})
			if err != nil {
				return errs.Wrap("r.presence.Current", err)
			}
			// A delayed removal of an old session must not remove a fresh rejoin.
			if current[c.UserID] == c.RoomID {
				return nil
			}
		}
		return errs.Wrap("r.media.Remove", r.media.Remove(ctx, c.RoomID, c.UserID))
	}
	return outbox.Permanent(fmt.Errorf("unknown media command %q", message.Kind))
}
