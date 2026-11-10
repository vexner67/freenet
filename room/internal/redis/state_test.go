package redis_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	roomredis "github.com/vexner67/freenet/room/internal/redis"
	"github.com/vexner67/freenet/room/internal/room"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }
func TestStateIntegration(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	c := &clock{time.Now().UTC().Truncate(time.Millisecond)}
	s := roomredis.NewState(client, c, 24*time.Hour)
	a, b, u, v := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	keys := []string{"room:live", "room:sweep-lock"}
	for _, id := range []uuid.UUID{a, b} {
		for _, prefix := range []string{"members", "joined", "empty", "closed", "playback", "chat"} {
			keys = append(keys, "room:"+prefix+":"+id.String())
		}
	}
	for _, id := range []uuid.UUID{u, v} {
		keys = append(keys, "room:current:"+id.String())
	}
	t.Cleanup(func() {
		if err := client.Del(context.Background(), keys...).Err(); err != nil {
			t.Error(err)
		}
	})
	change, err := s.Join(ctx, a, u, c.now, 1, 1)
	if err != nil || !change.Joined {
		t.Fatal(change, err)
	}
	if _, err = s.Join(ctx, a, v, c.now, 1, 1); !errors.Is(err, room.ErrFull) {
		t.Fatal(err)
	}
	if _, err = s.Join(ctx, b, v, c.now, 1, 1); !errors.Is(err, room.ErrTooManyRooms) {
		t.Fatal(err)
	}
	if _, err = s.Join(ctx, b, u, c.now, 1, 1); err != nil {
		t.Fatal(err)
	}
	current, err := s.Current(ctx, []uuid.UUID{u, v})
	if err != nil || current[u] != b || len(current) != 1 {
		t.Fatal(current, err)
	}
	old, _ := s.Members(ctx, a)
	if len(old) != 0 {
		t.Fatal("old room retained member")
	}
	p, err := room.NewPlayback(room.PlaybackInput{Kind: room.PlaybackYouTube, MediaID: "dQw4w9WgXcQ", Playing: true}, 1, c.now)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetPlayback(ctx, b, u, 0, p); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPlayback(ctx, b, u, 0, p); !errors.Is(err, room.ErrStalePlayback) {
		t.Fatal(err)
	}
	stored, err := s.Playback(ctx, b)
	if err != nil || stored.MediaID() != p.MediaID() || stored.Version() != 1 {
		t.Fatal(stored, err)
	}
	cleared, err := room.NewPlayback(room.PlaybackInput{}, 2, c.now)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetPlayback(ctx, b, v, 1, cleared); !errors.Is(err, room.ErrNotMember) {
		t.Fatal("outsider changed screen", err)
	}
	if err = s.ResetPlayback(ctx, b, 1, cleared); err != nil {
		t.Fatal(err)
	}
	if err = s.ResetPlayback(ctx, b, 1, cleared); !errors.Is(err, room.ErrStalePlayback) {
		t.Fatal(err)
	}
	stored, err = s.Playback(ctx, b)
	if err != nil || stored.Kind() != room.PlaybackNone || stored.Version() != 2 {
		t.Fatal(stored, err)
	}
	for range 55 {
		chat, err := room.NewChat(b, u, uuid.Nil, "hello", c.now)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.AppendChat(ctx, b, chat); err != nil {
			t.Fatal(err)
		}
	}
	history, err := s.Chat(ctx, b)
	if err != nil || len(history) != 50 {
		t.Fatal(len(history), err)
	}
	for _, key := range []string{"room:members:" + b.String(), "room:joined:" + b.String(), "room:current:" + u.String(), "room:playback:" + b.String(), "room:chat:" + b.String(), "room:live"} {
		ttl, err := client.PTTL(ctx, key).Result()
		if err != nil || ttl <= 0 {
			t.Fatalf("%s: %v %v", key, ttl, err)
		}
	}
	c.now = c.now.Add(40 * time.Second)
	if err = s.Heartbeat(ctx, b, u, c.now); err != nil {
		t.Fatal(err)
	}
	_, expired, err := s.Expire(ctx, b, u, c.now.Add(-45*time.Second), c.now)
	if err != nil || expired {
		t.Fatal("fresh member removed", err)
	}
	c.now = c.now.Add(45 * time.Second)
	_, expired, err = s.Expire(ctx, b, u, c.now.Add(-45*time.Second), c.now)
	if err != nil || !expired {
		t.Fatal("stale member retained", err)
	}
	if err = s.Heartbeat(ctx, b, u, c.now); !errors.Is(err, room.ErrNotMember) {
		t.Fatal(err)
	}
	since, err := s.EmptySince(ctx, b)
	if err != nil || !since.Equal(c.now) {
		t.Fatal(since, err)
	}
	if _, err = s.Join(ctx, b, u, c.now, 5, 20); err != nil {
		t.Fatal(err)
	}
	removed, err := s.Close(ctx, b, c.now)
	if err != nil || len(removed) != 1 {
		t.Fatal(removed, err)
	}
	if _, err = s.Join(ctx, b, u, c.now, 5, 20); !errors.Is(err, room.ErrClosed) {
		t.Fatal(err)
	}
	current, err = s.Current(ctx, []uuid.UUID{u})
	if err != nil || len(current) != 0 {
		t.Fatal(current, err)
	}
	entered := 0
	if err = s.SweepLocked(ctx, func(ctx context.Context) error {
		entered++
		return s.SweepLocked(ctx, func(context.Context) error { entered++; return nil })
	}); err != nil || entered != 1 {
		t.Fatal(entered, err)
	}
}
