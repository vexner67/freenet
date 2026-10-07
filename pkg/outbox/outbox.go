// Package outbox delivers what one service has to tell another, reliably
// (the transactional outbox): the message is recorded in the service's own
// database in the transaction of the change that caused it, and a relay
// delivers it afterwards, retrying until the receiver takes it. A change
// and its message are saved together or not at all, and a receiver that
// is down delays messages instead of losing them.
//
// Messages go out one at a time in the order they were recorded; one that
// fails holds back those after it, so an undo (a like taken back) never
// overtakes what it undoes. For the same reason one relay per database
// delivers at a time. Delivery is at least once: receivers must accept a
// message twice.
package outbox

import (
	"context"
	"errors"
	"time"
)

// Message is a recorded message. Kind and Payload are the sender's own
// encoding of what to deliver.
type Message struct {
	ID        int64
	Kind      string
	Payload   []byte
	RequestID string
	Attempts  int
	DueAt     time.Time
	CreatedAt time.Time
}

// Queue is the outbox as the relay sees it. Its methods belong to one
// relay goroutine.
type Queue interface {
	// Head returns the oldest messages, at most limit, oldest first; none
	// while another relay delivers from the same outbox.
	Head(ctx context.Context, limit int) ([]Message, error)
	Delete(ctx context.Context, id int64) error
	// Postpone records a failed attempt and when to try again.
	Postpone(ctx context.Context, id int64, attempts int, dueAt time.Time) error
	// Wait returns when a message may have been added, or after d.
	Wait(ctx context.Context, d time.Duration) error
}

// Deliver hands a message over to its receiver. An error made with
// Permanent drops the message; any other one retries it later.
type Deliver func(ctx context.Context, m Message) error

// Permanent marks a delivery error that no retry can fix, such as a
// message the receiver rejects as invalid.
func Permanent(err error) error {
	return permanentError{err: err}
}

type permanentError struct {
	err error
}

func (e permanentError) Error() string {
	return "permanent: " + e.err.Error()
}

func (e permanentError) Unwrap() error {
	return e.err
}

// IsPermanent reports whether err was made with Permanent.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}
