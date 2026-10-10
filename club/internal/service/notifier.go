package service

import (
	"context"

	"github.com/google/uuid"
)

// Notifier records what the notification service has to hear about
// appointments. It is called inside the transaction of the change
// (TxManager), so the notification is recorded if and only if the change
// is; it is delivered afterwards, retried while the notification service
// is down.
type Notifier interface {
	// Appointed: userID was made an admin of the club.
	Appointed(ctx context.Context, clubID, userID uuid.UUID) error
	// Dismissed takes back the notification of the appointment.
	Dismissed(ctx context.Context, clubID, userID uuid.UUID) error
}

// TxManager runs fn in one transaction: everything it saves is kept if it
// returns nil and undone otherwise.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
