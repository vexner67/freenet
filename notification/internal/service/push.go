package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/notification/internal/push"
	"github.com/vexner67/freenet/pkg/errs"
)

// ErrPushOff: the service has no key to push with.
var ErrPushOff = errors.New("push notifications are off")

// ErrSubscriptionGone: the browser no longer takes pushes; its
// subscription is forgotten.
var ErrSubscriptionGone = errors.New("push subscription is gone")

type SubscriptionRepository interface {
	// Save keeps a subscription: a browser subscribed by someone else
	// moves to this user, who keeps only push.MaxSubscriptions newest.
	Save(ctx context.Context, s *push.Subscription) error
	// Delete forgets the browser of a user, if it is theirs.
	Delete(ctx context.Context, userID uuid.UUID, endpoint push.Endpoint) error
	// Forget forgets the browser, whoever it pushed to.
	Forget(ctx context.Context, endpoint push.Endpoint) error
	List(ctx context.Context, userID uuid.UUID) ([]*push.Subscription, error)
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
}

// Sender pushes a payload to a browser; ErrSubscriptionGone when it no
// longer takes them.
type Sender interface {
	Send(ctx context.Context, s *push.Subscription, p push.Payload) error
	// PublicKey is the application server key browsers subscribe with
	// (VAPID, base64url).
	PublicKey() string
}

// Actors names the people and communities that did something, those it
// knows; a failure leaves pushes without names.
type Actors interface {
	Actors(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]push.Actor, error)
}

// PushDeps are what PushService works with; a nil Sender keeps pushes off.
type PushDeps struct {
	Subscriptions SubscriptionRepository
	Sender        Sender
	Actors        Actors
	Logger        *slog.Logger
	// Queue is how many pushes may wait; more are dropped.
	Queue int
	// Timeout bounds the delivery of one push to all browsers of a user.
	Timeout time.Duration
}

// PushService keeps the browsers of users and pushes to them what happens
// while no page is open. Pushing is best effort, as live events are: it
// happens after the request, a full queue drops it and a failure is only
// logged.
type PushService struct {
	subscriptions SubscriptionRepository
	sender        Sender
	actors        Actors
	logger        *slog.Logger
	jobs          chan pushJob
	timeout       time.Duration
	now           func() time.Time
}

// pushJob is a push to the browsers of one user. ctx is of the request
// that caused it, without its end: its ID goes into the logs.
type pushJob struct {
	ctx         context.Context //nolint:containedctx // the request's values outlive it in the queue
	recipientID uuid.UUID
	payload     push.Payload
}

func NewPushService(d PushDeps) *PushService {
	return &PushService{
		subscriptions: d.Subscriptions,
		sender:        d.Sender,
		actors:        d.Actors,
		logger:        d.Logger,
		jobs:          make(chan pushJob, d.Queue),
		timeout:       d.Timeout,
		now:           time.Now,
	}
}

// PublicKey is what browsers subscribe with; ErrPushOff without one.
func (s *PushService) PublicKey() (string, error) {
	if s.sender == nil {
		return "", ErrPushOff
	}

	return s.sender.PublicKey(), nil
}

// Subscribe lets the browser at endpoint take the pushes of userID.
func (s *PushService) Subscribe(ctx context.Context, userID uuid.UUID, endpoint push.Endpoint, keys push.Keys) error {
	if s.sender == nil {
		return ErrPushOff
	}

	sub, err := push.NewSubscription(userID, endpoint, keys, s.now())
	if err != nil {
		return errs.Wrap("push.NewSubscription", err)
	}

	return errs.Wrap("s.subscriptions.Save", s.subscriptions.Save(ctx, sub))
}

// Unsubscribe stops pushing to the browser of userID; another user's
// browser is left alone.
func (s *PushService) Unsubscribe(ctx context.Context, userID uuid.UUID, endpoint push.Endpoint) error {
	return errs.Wrap("s.subscriptions.Delete", s.subscriptions.Delete(ctx, userID, endpoint))
}

// PurgeUser forgets the browsers of a user whose page was deleted for
// good.
func (s *PushService) PurgeUser(ctx context.Context, userID uuid.UUID) error {
	return errs.Wrap("s.subscriptions.DeleteByUser", s.subscriptions.DeleteByUser(ctx, userID))
}

// Notified pushes a new notification to its recipient.
func (s *PushService) Notified(ctx context.Context, n *notification.Notification) {
	e := n.Event()

	s.enqueue(ctx, e.RecipientID, push.Payload{
		Type:  push.TypeNotification,
		Kind:  string(e.Kind),
		Actor: push.Actor{ID: e.ActorID},
		ID:    n.ID(),
	})
}

// Messaged pushes a new message to its recipients.
func (s *PushService) Messaged(ctx context.Context, m push.Message) error {
	if err := m.Validate(); err != nil {
		return errs.Wrap("m.Validate", err)
	}

	p := push.Payload{
		Type:      push.TypeMessage,
		Actor:     push.Actor{ID: m.SenderID},
		ID:        m.ID,
		ChatID:    m.ChatID,
		ChatTitle: m.ChatTitle,
	}
	if m.ChatID != uuid.Nil {
		p.Type = push.TypeChat
	}

	for _, id := range m.RecipientIDs {
		if id != m.SenderID {
			s.enqueue(ctx, id, p)
		}
	}

	return nil
}

func (s *PushService) enqueue(ctx context.Context, recipientID uuid.UUID, p push.Payload) {
	if s.sender == nil {
		return
	}

	select {
	case s.jobs <- pushJob{ctx: context.WithoutCancel(ctx), recipientID: recipientID, payload: p}:
	default:
		s.logger.WarnContext(ctx, "push queue is full, push dropped", "user_id", recipientID)
	}
}

// Run pushes what was queued with workers at once until ctx ends.
func (s *PushService) Run(ctx context.Context, workers int) {
	if s.sender == nil {
		return
	}

	done := make(chan struct{})

	for range workers {
		go func() {
			defer func() { done <- struct{}{} }()

			for {
				select {
				case <-ctx.Done():
					return
				case j := <-s.jobs:
					s.deliver(j)
				}
			}
		}()
	}

	for range workers {
		<-done
	}
}

// deliver pushes a job to every browser of its recipient.
func (s *PushService) deliver(j pushJob) {
	ctx, cancel := context.WithTimeout(j.ctx, s.timeout)
	defer cancel()

	subs, err := s.subscriptions.List(ctx, j.recipientID)
	if err != nil {
		s.logger.ErrorContext(ctx, "list push subscriptions", "error", err, "user_id", j.recipientID)
		return
	}

	if len(subs) == 0 {
		return
	}

	p := j.payload
	if actors, err := s.actors.Actors(ctx, []uuid.UUID{p.Actor.ID}); err != nil {
		s.logger.WarnContext(ctx, "name the actor of a push", "error", err)
	} else if a, ok := actors[p.Actor.ID]; ok {
		p.Actor = a
	}

	for _, sub := range subs {
		err := s.sender.Send(ctx, sub, p)

		switch {
		case errors.Is(err, ErrSubscriptionGone):
			if err = s.subscriptions.Forget(ctx, sub.Endpoint()); err != nil {
				s.logger.ErrorContext(ctx, "forget push subscription", "error", err)
			}
		case err != nil:
			s.logger.WarnContext(ctx, "push", "error", err, "user_id", j.recipientID)
		}
	}
}
