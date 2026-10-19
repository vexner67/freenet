package service

import (
	"context"

	"github.com/google/uuid"
)

// Notifier records what the notification service has to hear about
// appointments and the checkmark. It is called inside the transaction of the change
// (TxManager), so the notification is recorded if and only if the change
// is; it is delivered afterwards, retried while the notification service
// is down.
type Notifier interface {
	// Appointed: userID was made an admin of the club.
	Appointed(ctx context.Context, clubID, userID uuid.UUID) error
	// Dismissed takes back the notification of the appointment.
	Dismissed(ctx context.Context, clubID, userID uuid.UUID) error
	// Verified: the club, owned by ownerID, got the checkmark of the
	// administrators of the site; Unverified takes the notification back.
	Verified(ctx context.Context, clubID, ownerID uuid.UUID) error
	Unverified(ctx context.Context, clubID, ownerID uuid.UUID) error
	// Deleted: the club is gone, and the notifications it caused or
	// that are about it go too.
	Deleted(ctx context.Context, clubID uuid.UUID) error
}

// Readers records what the post service has to hear about memberships:
// its news show a member the wall of the club. Like Notifier, it is called
// inside the transaction of the change and delivered afterwards. Joined
// and Left may be told twice. Closed and Opened tell it of a club that
// became closed ("Закрытая группа") or open again: the wall of a closed
// club is for its members.
type Readers interface {
	Joined(ctx context.Context, clubID, userID uuid.UUID) error
	Left(ctx context.Context, clubID, userID uuid.UUID) error
	Closed(ctx context.Context, clubID uuid.UUID) error
	Opened(ctx context.Context, clubID uuid.UUID) error
	// Deleted: the club is gone, its wall, topics, albums and photos go
	// with it.
	Deleted(ctx context.Context, clubID uuid.UUID) error
}

// TxManager runs fn in one transaction: everything it saves is kept if it
// returns nil and undone otherwise.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
