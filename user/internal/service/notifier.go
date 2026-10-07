package service

import (
	"context"

	"github.com/google/uuid"
)

// Notifier records what the notification service has to hear about
// follows. It is called inside the transaction of the change (TxManager),
// so the notification is recorded if and only if the change is; it is
// delivered afterwards, retried while the notification service is down.
type Notifier interface {
	Followed(ctx context.Context, followerID, followeeID uuid.UUID) error
	// Unfollowed takes back the notification of the follow.
	Unfollowed(ctx context.Context, followerID, followeeID uuid.UUID) error
}

// TxManager runs fn in one transaction: everything it saves is kept if it
// returns nil and undone otherwise.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
