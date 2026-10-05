package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type notificationPostJSON struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
}

type notificationCommentJSON struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type notificationJSON struct {
	ID        string                   `json:"id"`
	Type      string                   `json:"type"`
	CreatedAt string                   `json:"createdAt"`
	Actor     authorJSON               `json:"actor"`
	Post      *notificationPostJSON    `json:"post"`
	Comment   *notificationCommentJSON `json:"comment"`
}

type notificationPageResponse struct {
	Notifications []notificationJSON `json:"notifications"`
	NextCursor    string             `json:"nextCursor"`
	// NewestID is the newest notification of the page, shown or not: the
	// one to mark read, so a left-out one does not stay unread forever.
	NewestID string `json:"newestId"`
}

type unreadCountResponse struct {
	Count int `json:"count"`
}

type markReadRequest struct {
	UpTo string `json:"upTo"`
}

// targets are what a page of notifications refers to, by ID; what was
// deleted meanwhile is missing.
type targets struct {
	actors   map[string]model.Author
	posts    map[string]model.Post
	comments map[string]model.Comment
}

// listNotifications is one page of the caller's notifications with who
// did it and to which post, each kind of thing looked up in one request.
func (a *api) listNotifications(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	unread, err := notificationBox(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.notifications.List(r.Context(), token, unread, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	t, err := a.notificationTargets(r, page.Notifications)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := notificationPageResponse{
		Notifications: make([]notificationJSON, 0, len(page.Notifications)),
		NextCursor:    page.NextCursor,
	}

	if len(page.Notifications) > 0 {
		resp.NewestID = page.Notifications[0].ID
	}

	for _, n := range page.Notifications {
		if item, ok := toNotificationJSON(n, t); ok {
			resp.Notifications = append(resp.Notifications, item)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// notificationBox reads ?box=: unread (the default) or archive.
func notificationBox(r *http.Request) (bool, error) {
	switch r.URL.Query().Get("box") {
	case "", "unread":
		return true, nil
	case "archive":
		return false, nil
	default:
		return false, failure.New(failure.InvalidArgument, "box must be unread or archive")
	}
}

func (a *api) notificationTargets(r *http.Request, notifications []model.Notification) (targets, error) {
	var actorIDs, postIDs, commentIDs []string

	for _, n := range notifications {
		actorIDs = append(actorIDs, n.ActorID)
		if n.PostID != "" {
			postIDs = append(postIDs, n.PostID)
		}
		if n.CommentID != "" {
			commentIDs = append(commentIDs, n.CommentID)
		}
	}

	var (
		t   targets
		err error
	)

	if t.actors, err = a.authors(r, actorIDs); err != nil {
		return targets{}, err
	}

	t.posts = make(map[string]model.Post)
	if len(postIDs) > 0 {
		posts, err := a.posts.GetMany(r.Context(), unique(postIDs))
		if err != nil {
			return targets{}, err
		}
		for _, p := range posts {
			t.posts[p.ID] = p
		}
	}

	t.comments = make(map[string]model.Comment)
	if len(commentIDs) > 0 {
		comments, err := a.posts.CommentsByID(r.Context(), unique(commentIDs))
		if err != nil {
			return targets{}, err
		}
		for _, c := range comments {
			t.comments[c.ID] = c
		}
	}

	return t, nil
}

// toNotificationJSON is false when the actor, the post or the comment is
// gone: such a notification has nothing left to show.
func toNotificationJSON(n model.Notification, t targets) (notificationJSON, bool) {
	actor, ok := t.actors[n.ActorID]
	if !ok {
		return notificationJSON{}, false
	}

	item := notificationJSON{
		ID:        n.ID,
		Type:      n.Kind,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
		Actor:     toAuthorJSON(actor),
	}

	if n.PostID != "" {
		p, ok := t.posts[n.PostID]
		if !ok {
			return notificationJSON{}, false
		}
		item.Post = &notificationPostJSON{ID: p.ID, CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339)}
	}

	if n.CommentID != "" {
		c, ok := t.comments[n.CommentID]
		if !ok {
			return notificationJSON{}, false
		}
		item.Comment = &notificationCommentJSON{ID: c.ID, Text: c.Text}
	}

	return item, true
}

func unique(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))

	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}

	return out
}

// countUnreadNotifications is for the counter next to "Мои Ответы".
func (a *api) countUnreadNotifications(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	count, err := a.notifications.CountUnread(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, unreadCountResponse{Count: count})
}

// markNotificationsRead moves the caller's notifications up to upTo, the
// newest one they have seen, to the archive.
func (a *api) markNotificationsRead(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req markReadRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.notifications.MarkRead(r.Context(), token, req.UpTo); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
