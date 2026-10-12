package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/inmem"
	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/notification/internal/service"
)

func TestCleanupService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := inmem.NewNotificationRepository()
	ann := uuid.New()

	// More old ones than a batch, and one of today.
	old := time.Now().Add(-31 * 24 * time.Hour)
	for i := range 1500 {
		id := notification.Before(old.Add(time.Duration(i) * time.Millisecond))
		n := notification.Restore(id, notification.Event{Kind: notification.KindFollow, RecipientID: ann, ActorID: uuid.New()}, old)

		if err := repo.Save(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	fresh, err := notification.New(notification.Event{Kind: notification.KindFollow, RecipientID: ann, ActorID: uuid.New()}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, fresh); err != nil {
		t.Fatal(err)
	}

	// A retention longer than their age keeps them all.
	if n, err := service.NewCleanupService(repo, 32*24*time.Hour).DeleteOld(ctx); err != nil || n != 0 {
		t.Fatalf("DeleteOld of a longer retention = %d, %v", n, err)
	}

	cleanup := service.NewCleanupService(repo, 30*24*time.Hour)
	if n, err := cleanup.DeleteOld(ctx); err != nil || n != 1500 {
		t.Fatalf("DeleteOld = %d, %v; want every old one", n, err)
	}
	if n, err := cleanup.DeleteOld(ctx); err != nil || n != 0 {
		t.Fatalf("DeleteOld again = %d, %v", n, err)
	}

	left, err := repo.List(ctx, service.Query{RecipientID: ann, Unread: true, Limit: 10})
	if err != nil || len(left) != 1 || left[0].ID() != fresh.ID() {
		t.Fatalf("left %v, %v; want the one of today", left, err)
	}
}
