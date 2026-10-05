package service

import (
	"context"

	"github.com/google/uuid"
)

// Notifier tells the notification service about follows. Delivery is best
// effort: a lost notification must not fail the follow that caused it, so
// the methods return nothing and implementations handle their failures
// themselves.
type Notifier interface {
	Followed(ctx context.Context, followerID, followeeID uuid.UUID)
	// Unfollowed takes back the notification of the follow.
	Unfollowed(ctx context.Context, followerID, followeeID uuid.UUID)
}
