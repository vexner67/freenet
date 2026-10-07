package outbox

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/vexner67/freenet/pkg/reqid"
)

// Memory is an outbox in process memory, for STORAGE=inmem: it is lost on
// restart, and as inmem storage has no transactions a message is recorded
// even if the change that caused it fails later.
type Memory struct {
	mu       sync.Mutex
	messages []Message
	lastID   int64
	wake     chan struct{}
	now      func() time.Time
}

func NewMemory() *Memory {
	return &Memory{
		wake: make(chan struct{}, 1),
		now:  time.Now,
	}
}

func (q *Memory) Add(ctx context.Context, kind string, payload []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.lastID++
	now := q.now()
	q.messages = append(q.messages, Message{
		ID:        q.lastID,
		Kind:      kind,
		Payload:   slices.Clone(payload),
		RequestID: reqid.FromContext(ctx),
		DueAt:     now,
		CreatedAt: now,
	})

	select {
	case q.wake <- struct{}{}:
	default:
	}

	return nil
}

func (q *Memory) Head(_ context.Context, limit int) ([]Message, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	return slices.Clone(q.messages[:min(limit, len(q.messages))]), nil
}

func (q *Memory) Delete(_ context.Context, id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.messages = slices.DeleteFunc(q.messages, func(m Message) bool { return m.ID == id })

	return nil
}

func (q *Memory) Postpone(_ context.Context, id int64, attempts int, dueAt time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := range q.messages {
		if q.messages[i].ID == id {
			q.messages[i].Attempts = attempts
			q.messages[i].DueAt = dueAt
		}
	}

	return nil
}

func (q *Memory) Wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-q.wake:
	case <-t.C:
	}

	return nil
}
