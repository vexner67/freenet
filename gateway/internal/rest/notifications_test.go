package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type notificationPage struct {
	Notifications []struct {
		ID        string         `json:"id"`
		Type      string         `json:"type"`
		CreatedAt string         `json:"createdAt"`
		Actor     map[string]any `json:"actor"`
		Post      *struct {
			ID        string `json:"id"`
			CreatedAt string `json:"createdAt"`
			Text      string `json:"text"`
		} `json:"post"`
		Comment *struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"comment"`
		Topic *struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"topic"`
		Photo *struct {
			ID    string `json:"id"`
			Small string `json:"small"`
		} `json:"photo"`
		Note *struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"note"`
	} `json:"notifications"`
	NextCursor string `json:"nextCursor"`
	NewestID   string `json:"newestId"`
}

func TestListNotifications(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/notifications?cursor=n9&limit=5", "", "Authorization", "Bearer tok")
	got := decode[notificationPage](t, w)

	if w.Code != http.StatusOK || got.NextCursor != "n-2" || got.NewestID != "n4" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if h.notifications.token != "tok" || !h.notifications.unread || h.notifications.cursor != "n9" || h.notifications.limit != 5 {
		t.Fatalf("request = %+v; unread is the default box", h.notifications)
	}

	// The follow by a vanished account, the like of a deleted post, the
	// message in a deleted topic, the like of a deleted photo and the
	// comments under a deleted note and deleted themselves are left out.
	if len(got.Notifications) != 17 {
		t.Fatalf("notifications = %+v", got.Notifications)
	}

	// A heart on a message names the topic, a heart and a comment on a
	// photo the photo.
	heart, photoLike, photoComment := got.Notifications[11], got.Notifications[12], got.Notifications[13]
	if heart.Type != "topic_message_like" || heart.Topic == nil || heart.Topic.ID != "t1" || heart.Post != nil ||
		heart.Comment == nil || heart.Comment.ID != "m1" || heart.Comment.Text != "Привет" {
		t.Fatalf("heart on a topic message = %+v", heart)
	}
	for _, n := range got.Notifications[12:14] {
		if n.Photo == nil || n.Photo.ID != "ph6" || n.Photo.Small != "/media/photos/u2/ph6/small.jpg" {
			t.Fatalf("%s names the photo %+v", n.Type, n.Photo)
		}
	}
	if photoLike.Type != "photo_like" || photoLike.Comment != nil || photoComment.Type != "photo_comment" || photoComment.Post != nil ||
		photoComment.Comment == nil || photoComment.Comment.ID != "pc1" || photoComment.Comment.Text != "красиво" {
		t.Fatalf("photo notifications = %+v, %+v", photoLike, photoComment)
	}

	// A mention under a photo names the photo and quotes the comment.
	if m := got.Notifications[15]; m.Type != "photo_mention" || m.Photo == nil || m.Photo.ID != "ph6" || m.Post != nil ||
		m.Comment == nil || m.Comment.ID != "pc1" || m.Comment.Text != "красиво" {
		t.Fatalf("mention under a photo = %+v", m)
	}

	// A comment under a note names the note and quotes the comment.
	if c := got.Notifications[16]; c.Type != "note_comment" || c.Note == nil || c.Note.ID != "note1" || c.Note.Title != "Про котов" ||
		c.Post != nil || c.Comment == nil || c.Comment.ID != "nc1" || c.Comment.Text != "мяу" {
		t.Fatalf("comment under a note = %+v", c)
	}

	// A mention names the post, the post and the comment, or the topic.
	inPost, inComment, inTopic := got.Notifications[8], got.Notifications[9], got.Notifications[10]
	if inPost.Type != "post_mention" || inPost.Actor["nickname"] != "bob" || inPost.Post == nil || inPost.Post.Text != "первая" || inPost.Comment != nil {
		t.Fatalf("mention in a post = %+v", inPost)
	}
	if inComment.Type != "comment_mention" || inComment.Post == nil || inComment.Comment == nil || inComment.Comment.Text != "согласен" {
		t.Fatalf("mention in a comment = %+v", inComment)
	}
	if inTopic.Type != "topic_mention" || inTopic.Topic == nil || inTopic.Topic.Title != "Знакомство" || inTopic.Post != nil || inTopic.Comment == nil {
		t.Fatalf("mention in a topic = %+v", inTopic)
	}

	comment, like, follow, wallPost := got.Notifications[0], got.Notifications[1], got.Notifications[2], got.Notifications[3]
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
	if wallPost.Type != "wall_post" || wallPost.Post == nil || wallPost.Post.Text != "первая" || wallPost.Comment != nil {
		t.Fatalf("wall post = %+v", wallPost)
	}
	if repost := got.Notifications[4]; repost.Type != "repost" || repost.Post == nil || repost.Post.ID != "p1" {
		t.Fatalf("repost = %+v", repost)
	}
	// A suggestion is no post: the community is all there is to show.
	if suggested := got.Notifications[5]; suggested.Type != "suggested_post" || suggested.Actor["name"] != "Кошки" || suggested.Post != nil {
		t.Fatalf("suggested post = %+v", suggested)
	}

	// A message in a topic names the topic: neither a post nor a comment.
	if written := got.Notifications[6]; written.Type != "topic_message" || written.Actor["nickname"] != "bob" || written.Topic == nil ||
		written.Topic.ID != "t1" || written.Topic.Title != "Знакомство" || written.Post != nil ||
		written.Comment == nil || written.Comment.Text != "Привет" {
		t.Fatalf("topic message = %+v", written)
	}
	// An appointment names only the community.
	if appointed := got.Notifications[7]; appointed.Type != "club_admin" || appointed.Actor["name"] != "Кошки" || appointed.Post != nil || appointed.Topic != nil {
		t.Fatalf("club admin = %+v", appointed)
	}

	// So does a reminder of an event: the event is the actor.
	if reminder := got.Notifications[14]; reminder.Type != "event_reminder" || reminder.Actor["name"] != "Кошки" || reminder.Post != nil {
		t.Fatalf("event reminder = %+v", reminder)
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
