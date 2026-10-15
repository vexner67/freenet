package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/inmem"
	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/notification/internal/push"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/pkg/pubsub"
)

// sent is one push the fake sender made.
type sent struct {
	endpoint push.Endpoint
	payload  push.Payload
}

// fakeSender hands every push to the test; the browsers in gone no
// longer take them.
type fakeSender struct {
	pushes chan sent
	gone   map[push.Endpoint]bool
}

func (f *fakeSender) Send(_ context.Context, s *push.Subscription, p push.Payload) error {
	f.pushes <- sent{endpoint: s.Endpoint(), payload: p}

	if f.gone[s.Endpoint()] {
		return service.ErrSubscriptionGone
	}

	return nil
}

func (f *fakeSender) PublicKey() string {
	return "key"
}

type fakeActors map[uuid.UUID]push.Actor

func (f fakeActors) Actors(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]push.Actor, error) {
	found := make(map[uuid.UUID]push.Actor)
	for _, id := range ids {
		if a, ok := f[id]; ok {
			found[id] = a
		}
	}

	return found, nil
}

// forgetting tells the test which browsers it forgot.
type forgetting struct {
	*inmem.SubscriptionRepository
	forgot chan push.Endpoint
}

func (f forgetting) Forget(ctx context.Context, endpoint push.Endpoint) error {
	err := f.SubscriptionRepository.Forget(ctx, endpoint)
	f.forgot <- endpoint

	return err
}

type pushFixture struct {
	subs          forgetting
	sender        *fakeSender
	pushes        *service.PushService
	notifications *service.NotificationService
}

// newPushFixture runs the push queue, one job at a time, until the test
// ends.
func newPushFixture(t *testing.T, actors fakeActors) pushFixture {
	t.Helper()

	f := pushFixture{
		subs:   forgetting{SubscriptionRepository: inmem.NewSubscriptionRepository(), forgot: make(chan push.Endpoint, 16)},
		sender: &fakeSender{pushes: make(chan sent, 16), gone: make(map[push.Endpoint]bool)},
	}
	f.pushes = service.NewPushService(service.PushDeps{
		Subscriptions: f.subs,
		Sender:        f.sender,
		Actors:        actors,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Queue:         16,
		Timeout:       time.Second,
	})
	f.notifications = service.NewNotificationService(inmem.NewNotificationRepository(), inmem.NewReadMarkRepository(), pubsub.New[uuid.UUID, service.Change](64), f.pushes)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		f.pushes.Run(ctx, 1)
		close(done)
	}()

	t.Cleanup(func() {
		cancel()
		<-done
	})

	return f
}

func (f pushFixture) next(t *testing.T) sent {
	t.Helper()

	select {
	case s := <-f.sender.pushes:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("no push")
		return sent{}
	}
}

func keys(t *testing.T) push.Keys {
	t.Helper()

	k, err := push.NewKeys("BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4", "BTBZMqHH6r4Tts7J_aSIgg==")
	if err != nil {
		t.Fatal(err)
	}

	return k
}

func subscribe(t *testing.T, s *service.PushService, userID uuid.UUID, endpoint push.Endpoint) {
	t.Helper()

	if err := s.Subscribe(context.Background(), userID, endpoint, keys(t)); err != nil {
		t.Fatal(err)
	}
}

func TestPush_Off(t *testing.T) {
	t.Parallel()

	s := service.NewPushService(service.PushDeps{Subscriptions: inmem.NewSubscriptionRepository()})

	if _, err := s.PublicKey(); !errors.Is(err, service.ErrPushOff) {
		t.Fatalf("PublicKey: %v, want ErrPushOff", err)
	}

	if err := s.Subscribe(context.Background(), uuid.New(), "https://push.example/1", keys(t)); !errors.Is(err, service.ErrPushOff) {
		t.Fatalf("Subscribe: %v, want ErrPushOff", err)
	}
}

func TestPush_Notified(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	f := newPushFixture(t, fakeActors{bob: {ID: bob, Name: "Боб Петров", Gender: "male", Nickname: "bob"}})
	subscribe(t, f.pushes, ann, "https://push.example/ann")

	if err := f.notifications.Notify(context.Background(), like(ann, bob, uuid.New())); err != nil {
		t.Fatal(err)
	}

	got := f.next(t)
	if got.endpoint != "https://push.example/ann" || got.payload.Type != push.TypeNotification || got.payload.Kind != "like" ||
		got.payload.Actor.Name != "Боб Петров" || got.payload.ID == uuid.Nil {
		t.Fatalf("push = %+v", got)
	}

	// The same like delivered again by the outbox is not pushed again: the
	// queue runs one job at a time, so the next push is the follow.
	post := uuid.New()
	for _, e := range []notification.Event{like(ann, bob, post), like(ann, bob, post), follow(ann, bob)} {
		if err := f.notifications.Notify(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}

	if got = f.next(t); got.payload.Kind != "like" {
		t.Fatalf("push = %+v", got)
	}
	if got = f.next(t); got.payload.Kind != "follow" {
		t.Fatalf("push after the repeated like = %+v", got)
	}
}

func TestPush_Messaged(t *testing.T) {
	t.Parallel()

	ann, bob, eve := uuid.New(), uuid.New(), uuid.New()
	f := newPushFixture(t, fakeActors{})
	subscribe(t, f.pushes, ann, "https://push.example/ann")
	subscribe(t, f.pushes, bob, "https://push.example/bob")

	chat := uuid.New()
	m := push.Message{ID: uuid.New(), SenderID: bob, RecipientIDs: []uuid.UUID{ann, bob, eve}, ChatID: chat, ChatTitle: "Поход"}

	if err := f.pushes.Messaged(context.Background(), m); err != nil {
		t.Fatal(err)
	}

	// Only ann: bob sent it and eve has no browser.
	got := f.next(t)
	if got.endpoint != "https://push.example/ann" || got.payload.Type != push.TypeChat || got.payload.ChatID != chat ||
		got.payload.ChatTitle != "Поход" || got.payload.Actor.ID != bob {
		t.Fatalf("push = %+v", got)
	}

	// The queue runs one job at a time: had bob or eve got one, it would
	// come before this one.
	again := push.Message{ID: uuid.New(), SenderID: bob, RecipientIDs: []uuid.UUID{ann}}
	if err := f.pushes.Messaged(context.Background(), again); err != nil {
		t.Fatal(err)
	}

	if got = f.next(t); got.endpoint != "https://push.example/ann" || got.payload.ID != again.ID {
		t.Fatalf("next push = %+v", got)
	}

	if err := f.pushes.Messaged(context.Background(), push.Message{SenderID: bob}); !errors.Is(err, push.ErrNoRecipients) {
		t.Fatalf("without recipients: %v", err)
	}
}

func TestPush_GoneForgotten(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	f := newPushFixture(t, fakeActors{})
	subscribe(t, f.pushes, ann, "https://push.example/old")
	f.sender.gone["https://push.example/old"] = true

	if err := f.pushes.Messaged(context.Background(), push.Message{ID: uuid.New(), SenderID: bob, RecipientIDs: []uuid.UUID{ann}}); err != nil {
		t.Fatal(err)
	}

	f.next(t)

	select {
	case endpoint := <-f.subs.forgot:
		if endpoint != "https://push.example/old" {
			t.Fatalf("forgot %s", endpoint)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the gone subscription is kept")
	}
}

func TestPush_SubscriptionsOfUsers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	f := newPushFixture(t, fakeActors{})

	subscribe(t, f.pushes, ann, "https://push.example/shared")
	// Bob signs in on the same browser: it pushes to him now.
	subscribe(t, f.pushes, bob, "https://push.example/shared")

	if subs, _ := f.subs.List(ctx, ann); len(subs) != 0 {
		t.Fatalf("ann keeps %d browsers", len(subs))
	}

	// Ann cannot unsubscribe bob's browser.
	if err := f.pushes.Unsubscribe(ctx, ann, "https://push.example/shared"); err != nil {
		t.Fatal(err)
	}

	if subs, _ := f.subs.List(ctx, bob); len(subs) != 1 {
		t.Fatalf("bob has %d browsers", len(subs))
	}

	for i := range push.MaxSubscriptions + 2 {
		subscribe(t, f.pushes, ann, push.Endpoint("https://push.example/"+string(rune('a'+i))))
	}

	subs, err := f.subs.List(ctx, ann)
	if err != nil || len(subs) != push.MaxSubscriptions || subs[0].Endpoint() != "https://push.example/l" {
		t.Fatalf("ann's browsers: %d, %v", len(subs), err)
	}

	if err = f.notifications.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}

	if subs, _ = f.subs.List(ctx, ann); len(subs) != 0 {
		t.Fatalf("a purged user keeps %d browsers", len(subs))
	}

}
