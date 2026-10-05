package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type notificationPage struct {
	Notifications []struct {
		ID        string            `json:"id"`
		Type      string            `json:"type"`
		CreatedAt string            `json:"createdAt"`
		Actor     map[string]string `json:"actor"`
		Post      *struct {
			ID        string `json:"id"`
			CreatedAt string `json:"createdAt"`
		} `json:"post"`
		Comment *struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"comment"`
	} `json:"notifications"`
	NextCursor string `json:"nextCursor"`
	NewestID   string `json:"newestId"`
}

func TestListNotifications(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/notifications?cursor=n9&limit=5", "", "Authorization", "Bearer tok")
	got := decode[notificationPage](t, w)

	if w.Code != http.StatusOK || got.NextCursor != "n0" || got.NewestID != "n4" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if h.notifications.token != "tok" || !h.notifications.unread || h.notifications.cursor != "n9" || h.notifications.limit != 5 {
		t.Fatalf("request = %+v; unread is the default box", h.notifications)
	}

	// The follow by a vanished account and the like of a deleted post are
	// left out.
	if len(got.Notifications) != 3 {
		t.Fatalf("notifications = %+v", got.Notifications)
	}

	comment, like, follow := got.Notifications[0], got.Notifications[1], got.Notifications[2]
	if comment.Type != "comment" || comment.Actor["nickname"] != "bob" || comment.Comment == nil || comment.Comment.Text != "согласен" ||
		comment.Post == nil || comment.Post.CreatedAt != "2026-10-04T15:30:00Z" {
		t.Fatalf("comment = %+v", comment)
	}
	if like.Type != "like" || like.Post == nil || like.Post.ID != "p1" || like.Comment != nil {
		t.Fatalf("like = %+v", like)
	}
	if follow.Type != "follow" || follow.Post != nil || follow.Comment != nil || follow.CreatedAt != "2026-10-05T18:00:00Z" {
		t.Fatalf("follow = %+v", follow)
	}

	archive := newHarness(rest.Config{})
	if w = archive.do("GET", "/api/v1/notifications?box=archive", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || archive.notifications.unread {
		t.Fatalf("archive: %d, unread = %v", w.Code, archive.notifications.unread)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/notifications?box=spam", "", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown box: %d", w.Code)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/notifications", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestCountUnreadNotifications(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/notifications/unread-count", "", "Authorization", "Bearer tok")

	if got := decode[map[string]int](t, w); w.Code != http.StatusOK || got["count"] != 3 || h.notifications.token != "tok" {
		t.Fatalf("%d %v", w.Code, got)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/notifications/unread-count", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestMarkNotificationsRead(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/notifications/read", `{"upTo":"n4"}`, "Authorization", "Bearer tok")

	if w.Code != http.StatusNoContent || h.notifications.markedUpTo != "n4" || h.notifications.token != "tok" {
		t.Fatalf("%d, marked up to %q", w.Code, h.notifications.markedUpTo)
	}

	h = newHarness(rest.Config{})
	h.notifications.err = failure.New(failure.NotFound, "notification not found")
	if w = h.do("POST", "/api/v1/notifications/read", `{"upTo":"someone-else"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("someone else's notification: %d", w.Code)
	}

	if w = newHarness(rest.Config{}).do("POST", "/api/v1/notifications/read", `{"upTo":"n4"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}
