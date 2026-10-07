package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/inmem"
	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/pkg/pubsub"
)

func newService() *service.NotificationService {
	return service.NewNotificationService(inmem.NewNotificationRepository(), inmem.NewReadMarkRepository(), pubsub.New[uuid.UUID, service.Change](64))
}

func like(recipient, actor, post uuid.UUID) notification.Event {
	return notification.Event{Kind: notification.KindLike, RecipientID: recipient, ActorID: actor, PostID: post}
}

func comment(recipient, actor, post, comment uuid.UUID) notification.Event {
	return notification.Event{Kind: notification.KindComment, RecipientID: recipient, ActorID: actor, PostID: post, CommentID: comment}
}

func follow(recipient, actor uuid.UUID) notification.Event {
	return notification.Event{Kind: notification.KindFollow, RecipientID: recipient, ActorID: actor}
}

func notify(t *testing.T, svc *service.NotificationService, events ...notification.Event) {
	t.Helper()

	for _, e := range events {
		if err := svc.Notify(context.Background(), e); err != nil {
			t.Fatalf("Notify(%+v): %v", e, err)
		}
	}
}

func list(t *testing.T, svc *service.NotificationService, userID uuid.UUID, unread bool) []notification.Event {
	t.Helper()

	page, err := svc.List(context.Background(), userID, unread, uuid.Nil, service.MaxPageSize)
	if err != nil {
		t.Fatal(err)
	}

	events := make([]notification.Event, len(page.Notifications))
	for i, n := range page.Notifications {
		events[i] = n.Event()
	}

	return events
}

func count(t *testing.T, svc *service.NotificationService, userID uuid.UUID) int {
	t.Helper()

	n, err := svc.CountUnread(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}

	return n
}

func TestNotify(t *testing.T) {
	t.Parallel()

	svc := newService()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	post := uuid.New()

	notify(t, svc,
		follow(ann, bob),
		like(ann, bob, post),
		like(ann, bob, post), // a repeated like is notified once
		comment(ann, cat, post, uuid.New()),
		follow(bob, ann),
	)

	got := list(t, svc, ann, true)
	want := []notification.Kind{notification.KindComment, notification.KindLike, notification.KindFollow}
	if len(got) != len(want) {
		t.Fatalf("ann's notifications = %+v", got)
	}
	for i := range want {
		if got[i].Kind != want[i] {
			t.Fatalf("newest first: %+v", got)
		}
	}

	if n := count(t, svc, ann); n != 3 {
		t.Fatalf("unread = %d, want 3", n)
	}

	if err := svc.Notify(context.Background(), like(ann, ann, post)); !errors.Is(err, notification.ErrActorRecipient) {
		t.Fatalf("own like: %v", err)
	}
}

func TestRetract(t *testing.T) {
	t.Parallel()

	svc := newService()
	ann, bob := uuid.New(), uuid.New()
	post, commentID := uuid.New(), uuid.New()

	notify(t, svc, like(ann, bob, post), comment(ann, bob, post, commentID), follow(ann, bob))

	for _, e := range []notification.Event{like(ann, bob, post), comment(ann, bob, post, commentID), follow(ann, bob)} {
		if err := svc.Retract(context.Background(), e); err != nil {
			t.Fatalf("Retract(%+v): %v", e, err)
		}
	}

	if got := list(t, svc, ann, true); len(got) != 0 {
		t.Fatalf("after retracting: %+v", got)
	}

	if err := svc.Retract(context.Background(), follow(ann, bob)); err != nil {
		t.Fatalf("nothing to retract is fine: %v", err)
	}

	// Taken back and done again, a like is notified again.
	notify(t, svc, like(ann, bob, post))
	if n := count(t, svc, ann); n != 1 {
		t.Fatalf("unread = %d, want 1", n)
	}
}

func TestForgetPost(t *testing.T) {
	t.Parallel()

	svc := newService()
	ann, bob := uuid.New(), uuid.New()
	gone, kept := uuid.New(), uuid.New()

	notify(t, svc, like(ann, bob, gone), comment(ann, bob, gone, uuid.New()), like(ann, bob, kept), follow(ann, bob))

	if err := svc.ForgetPost(context.Background(), gone); err != nil {
		t.Fatal(err)
	}

	got := list(t, svc, ann, true)
	if len(got) != 2 || got[0].Kind != notification.KindFollow || got[1].PostID != kept {
		t.Fatalf("after forgetting the post: %+v", got)
	}
}

func TestPurgeUser(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	// Ann got some and caused some; bob and cat have their own as well.
	notify(t, svc, follow(ann, bob), like(bob, ann, uuid.New()), follow(cat, bob), like(cat, ann, uuid.New()))

	page, err := svc.List(ctx, ann, true, uuid.Nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.MarkRead(ctx, ann, page.Notifications[0].ID()); err != nil {
		t.Fatal(err)
	}

	if err = svc.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if err = svc.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("purge twice: %v", err)
	}

	if got := list(t, svc, ann, false); len(got) != 0 {
		t.Fatalf("ann's notifications left: %+v", got)
	}
	if got := list(t, svc, bob, true); len(got) != 0 {
		t.Fatalf("bob still hears of ann's like: %+v", got)
	}
	if got := list(t, svc, cat, true); len(got) != 1 || got[0].ActorID != bob {
		t.Fatalf("cat's = %+v, want bob's follow only", got)
	}

	// The read mark went too: a new notification for the same ID is unread.
	notify(t, svc, follow(ann, cat))
	if n := count(t, svc, ann); n != 1 {
		t.Fatalf("unread after the purge = %d, want 1", n)
	}
}

func TestReadMark(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	notify(t, svc, follow(ann, bob), follow(ann, cat))

	unread, err := svc.List(ctx, ann, true, uuid.Nil, 20)
	if err != nil || len(unread.Notifications) != 2 {
		t.Fatalf("unread = %+v, %v", unread, err)
	}
	if got := list(t, svc, ann, false); len(got) != 0 {
		t.Fatalf("archive before reading = %+v", got)
	}

	// Reading the newest moves everything to the archive.
	if err = svc.MarkRead(ctx, ann, unread.Notifications[0].ID()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, svc, ann); n != 0 {
		t.Fatalf("unread after reading = %d", n)
	}
	if got := list(t, svc, ann, false); len(got) != 2 {
		t.Fatalf("archive = %+v", got)
	}

	// An older mark from another tab does not bring them back.
	if err = svc.MarkRead(ctx, ann, unread.Notifications[1].ID()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, svc, ann); n != 0 {
		t.Fatalf("the mark moved back: unread = %d", n)
	}

	// New ones after the mark are unread.
	notify(t, svc, like(ann, bob, uuid.New()))
	if got := list(t, svc, ann, true); len(got) != 1 || got[0].Kind != notification.KindLike {
		t.Fatalf("new after reading = %+v", got)
	}

	// Only your own notification can be the mark.
	bobs := list(t, svc, bob, true)
	if len(bobs) != 0 {
		t.Fatalf("bob has nothing: %+v", bobs)
	}
	if err = svc.MarkRead(ctx, bob, unread.Notifications[0].ID()); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("someone else's notification: %v", err)
	}
	if err = svc.MarkRead(ctx, ann, uuid.Max); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("a made-up ID: %v", err)
	}
}

func TestList_Paging(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann := uuid.New()

	for range 5 {
		notify(t, svc, follow(ann, uuid.New()))
	}

	var (
		pages  []int
		cursor uuid.UUID
		seen   = map[uuid.UUID]bool{}
	)

	for {
		page, err := svc.List(ctx, ann, true, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}

		pages = append(pages, len(page.Notifications))
		for _, n := range page.Notifications {
			if seen[n.ID()] {
				t.Fatalf("%s on two pages", n.ID())
			}
			seen[n.ID()] = true
		}

		if page.NextCursor == uuid.Nil {
			break
		}
		cursor = page.NextCursor
	}

	if len(pages) != 3 || pages[0] != 2 || pages[2] != 1 || len(seen) != 5 {
		t.Fatalf("pages = %v", pages)
	}
}

func TestCountUnread_Capped(t *testing.T) {
	t.Parallel()

	svc := newService()
	ann := uuid.New()

	for range service.MaxUnreadCount + 5 {
		notify(t, svc, follow(ann, uuid.New()))
	}

	if n := count(t, svc, ann); n != service.MaxUnreadCount {
		t.Fatalf("unread = %d, want the cap %d", n, service.MaxUnreadCount)
	}
}

func TestNotificationService_Events(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob := uuid.New(), uuid.New()

	changes, cancel := svc.Subscribe(ann)
	defer cancel()

	follow := notification.Event{Kind: notification.KindFollow, RecipientID: ann, ActorID: bob}
	if err := svc.Notify(ctx, follow); err != nil {
		t.Fatal(err)
	}
	if c := <-changes; c != service.ChangeNew {
		t.Fatalf("after Notify: %q, want new", c)
	}

	page, err := svc.List(ctx, ann, true, uuid.Nil, 10)
	if err != nil || len(page.Notifications) != 1 {
		t.Fatalf("List = %+v, %v", page, err)
	}
	if err = svc.MarkRead(ctx, ann, page.Notifications[0].ID()); err != nil {
		t.Fatal(err)
	}
	if c := <-changes; c != service.ChangeRead {
		t.Fatalf("after MarkRead: %q, want read", c)
	}

	if err = svc.Retract(ctx, follow); err != nil {
		t.Fatal(err)
	}
	if c := <-changes; c != service.ChangeGone {
		t.Fatalf("after Retract: %q, want gone", c)
	}

	// A notification that fails tells nobody.
	if err = svc.Notify(ctx, notification.Event{Kind: notification.KindFollow, RecipientID: ann, ActorID: ann}); err == nil {
		t.Fatal("nobody is notified about themselves")
	}
	select {
	case c := <-changes:
		t.Fatalf("unexpected change %q", c)
	default:
	}
}
