// Package pubsub delivers events to whoever is subscribed to a key (a
// user), within one process: the live updates a service streams to the
// gateway. Delivery is in order per subscriber and never blocks the
// publisher: a subscriber that falls behind is dropped — its channel is
// closed — and has to subscribe again and catch up from storage, rather
// than miss events without knowing.
package pubsub

import "sync"

// Hub is safe for concurrent use. The zero value is not usable: use New.
type Hub[K comparable, T any] struct {
	mu     sync.Mutex
	subs   map[K]map[*subscriber[T]]struct{}
	buffer int
}

type subscriber[T any] struct {
	ch     chan T
	closed bool
}

// New makes a hub whose subscribers may lag behind by buffer events.
func New[K comparable, T any](buffer int) *Hub[K, T] {
	return &Hub[K, T]{
		subs:   make(map[K]map[*subscriber[T]]struct{}),
		buffer: buffer,
	}
}

// Subscribe returns the events published for key from now on and a cancel
// func, which closes the channel and must be called when done. The channel
// is also closed when the subscriber falls behind.
func (h *Hub[K, T]) Subscribe(key K) (<-chan T, func()) {
	s := &subscriber[T]{ch: make(chan T, h.buffer)}

	h.mu.Lock()
	if h.subs[key] == nil {
		h.subs[key] = make(map[*subscriber[T]]struct{})
	}
	h.subs[key][s] = struct{}{}
	h.mu.Unlock()

	var once sync.Once

	return s.ch, func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.drop(key, s)
		})
	}
}

// Publish hands event to every subscriber of key without waiting.
func (h *Hub[K, T]) Publish(key K, event T) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for s := range h.subs[key] {
		select {
		case s.ch <- event:
		default:
			h.drop(key, s)
		}
	}
}

// Close drops every subscriber, so that streams end on shutdown.
func (h *Hub[K, T]) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for key, subs := range h.subs {
		for s := range subs {
			h.drop(key, s)
		}
	}
}

// drop removes and closes s; the caller holds the lock.
func (h *Hub[K, T]) drop(key K, s *subscriber[T]) {
	if s.closed {
		return
	}

	s.closed = true
	close(s.ch)

	delete(h.subs[key], s)
	if len(h.subs[key]) == 0 {
		delete(h.subs, key)
	}
}
