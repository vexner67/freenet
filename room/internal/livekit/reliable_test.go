package livekit_test

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/room/internal/inmem"
	"github.com/vexner67/freenet/room/internal/livekit"
	"github.com/vexner67/freenet/room/internal/service"
	"io"
	"log/slog"
	"testing"
	"time"
)

type recorder struct {
	messages []outbox.Message
	err      error
}

func (q *recorder) Add(_ context.Context, kind string, payload []byte) error {
	if q.err != nil {
		return q.err
	}
	q.messages = append(q.messages, outbox.Message{Kind: kind, Payload: payload})
	return nil
}

type failingMedia struct {
	service.MediaServer
	removed, deleted int
	err              error
}

func (m *failingMedia) Remove(context.Context, uuid.UUID, uuid.UUID) error { m.removed++; return m.err }
func (m *failingMedia) Delete(context.Context, uuid.UUID) error            { m.deleted++; return m.err }
func TestReliableMediaRemoval(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Now().UTC()
	c := clock{now}
	presence := inmem.NewState(c)
	bans := inmem.NewRepository()
	q := &recorder{}
	media := &failingMedia{err: errors.New("media unavailable")}
	reliable := livekit.NewReliable(media, q, presence, bans, c, slog.New(slog.NewTextHandler(io.Discard, nil)))
	id, user := uuid.New(), uuid.New()
	if err := reliable.Remove(ctx, id, user); err != nil || len(q.messages) != 1 || media.removed != 1 {
		t.Fatal(err, len(q.messages), media.removed)
	}
	if err := reliable.Deliver(ctx, q.messages[0]); err == nil || media.removed != 2 {
		t.Fatal("failed removal must retry", err)
	}
	// A normal leave can be delivered after the same user has joined again.
	if _, err := presence.Join(ctx, id, user, now, 5, 20); err != nil {
		t.Fatal(err)
	}
	if err := reliable.Deliver(ctx, q.messages[0]); err != nil || media.removed != 2 {
		t.Fatal("stale leave disconnected new session", err)
	}
	// A ban always wins over stale presence.
	if err := bans.SaveBan(ctx, id, user, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	media.err = nil
	if err := reliable.Deliver(ctx, q.messages[0]); err != nil || media.removed != 3 {
		t.Fatal(err, media.removed)
	}
	q.err = errors.New("outbox unavailable")
	if err := reliable.Remove(ctx, id, user); err == nil || media.removed != 3 {
		t.Fatal("mutation before durable record", err)
	}
	q.err = nil
	if err := reliable.Delete(ctx, id); err != nil || len(q.messages) != 2 || media.deleted != 1 {
		t.Fatal(err, media.deleted)
	}
	if err := reliable.Deliver(ctx, q.messages[1]); err != nil || media.deleted != 2 {
		t.Fatal(err, media.deleted)
	}
}
