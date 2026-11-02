package postgres_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/postgres"
	"github.com/vexner67/freenet/pkg/pgpool"
)

func TestFollowingChannels(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	following := postgres.NewFollowingRepository(pool)
	now := time.Now().UTC()

	// Of their own, so that runs do not meet.
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
	channel := fedi.ChannelURL("c" + suffix)
	account := fedi.URL("https://m.example/users/" + suffix)

	for _, actor := range []fedi.URL{channel, account} {
		f, err := fedi.NewFollowing(actor, false, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = following.Save(ctx, f); err != nil {
			t.Fatal(err)
		}
	}

	channels, err := following.Channels(ctx)
	if err != nil || !slices.Contains(channels, channel) || slices.Contains(channels, account) {
		t.Fatalf("Channels = %v, %v; want %s and not %s", channels, err, channel, account)
	}
}
