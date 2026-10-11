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
	// Text is what a post on the recipient's wall says, as openvk quotes it.
	Text string `json:"text"`
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
	// Topic is the discussion a topic_message or a topic_mention was
	// written in.
	Topic *notificationTopicJSON `json:"topic,omitempty"`
	// Photo is the photo of an album a photo_like or a photo_comment is
	// about.
	Photo *photoJSON `json:"photo,omitempty"`
	// Note is the note a note_comment was left under.
	Note *notificationNoteJSON `json:"note,omitempty"`
}

type notificationNoteJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type notificationTopicJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
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

// kindSuggestedPost names a suggestion, not a post, by PostID: a
// suggestion is not public and the community, the actor, is all its
// admins are told about ("новые записи в предложке" of openvk). The
// notification goes when the suggestion does.
const kindSuggestedPost = "suggested_post"

// kindTopicMessage names a topic by PostID and a message of it by
// CommentID, neither a post nor a comment: the notification shows the
// topic and goes with the message or the topic.
const kindTopicMessage = "topic_message"

// kindTopicMention names a topic and a message of it as kindTopicMessage
// does: the message names the recipient ("Вас упомянул пользователь … в
// обсуждении" of openvk).
const kindTopicMention = "topic_mention"

// kindTopicMessageLike names a topic and a message of it too: the actor
// liked the recipient's message.
const kindTopicMessageLike = "topic_message_like"

// kindPhotoLike, kindPhotoComment and kindPhotoMention name a photo of an
// album by PostID (and a comment under it by CommentID), not a post: the
// notification shows the photo and goes with it, the like or the comment.
// The comment of kindPhotoMention names the recipient.
const (
	kindPhotoLike    = "photo_like"
	kindPhotoComment = "photo_comment"
	kindPhotoMention = "photo_mention"
)

// kindNoteComment names a note by PostID and a comment under it by
// CommentID ("оставил комментарий под вашей заметкой" of openvk).
const kindNoteComment = "note_comment"

// namesTopic tells whether PostID of a notification of kind is a topic
// and its CommentID a message of it.
func namesTopic(kind string) bool {
	return kind == kindTopicMessage || kind == kindTopicMention || kind == kindTopicMessageLike
}

// namesPhoto tells whether PostID of a notification of kind is a photo of
// an album.
func namesPhoto(kind string) bool {
	return kind == kindPhotoLike || kind == kindPhotoComment || kind == kindPhotoMention
}

// namesPost tells whether PostID of a notification of kind is a post, and
// namesComment whether its CommentID is a comment.
func namesPost(kind string) bool {
	return kind != kindSuggestedPost && kind != kindNoteComment && !namesTopic(kind) && !namesPhoto(kind)
}

func namesComment(kind string) bool {
	return kind != kindNoteComment && !namesTopic(kind) && !namesPhoto(kind)
}

// targets are what a page of notifications refers to, by ID; what was
// deleted meanwhile is missing.
type targets struct {
	actors   map[string]model.Author
	posts    map[string]model.Post
	comments map[string]model.Comment
	topics   map[string]model.Topic
	photos   map[string]model.AlbumPhoto
	notes    map[string]model.Note
	// quotes are what the messages of topics and the comments of photos
	// and notes that notifications name say, by ID.
	quotes map[string]string
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

	t, err := a.notificationTargets(r, token, page.Notifications)
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

func (a *api) notificationTargets(r *http.Request, token string, notifications []model.Notification) (targets, error) {
	var actorIDs, postIDs, commentIDs, topicIDs, photoIDs, messageIDs, photoCommentIDs, noteIDs, noteCommentIDs []string

	for _, n := range notifications {
		actorIDs = append(actorIDs, n.ActorID)
		if n.PostID != "" && namesPost(n.Kind) {
			postIDs = append(postIDs, n.PostID)
		}
		if n.CommentID != "" && namesComment(n.Kind) {
			commentIDs = append(commentIDs, n.CommentID)
		}
		if namesTopic(n.Kind) {
			topicIDs = append(topicIDs, n.PostID)
			if n.CommentID != "" {
				messageIDs = append(messageIDs, n.CommentID)
			}
		}
		if namesPhoto(n.Kind) {
			photoIDs = append(photoIDs, n.PostID)
			if n.CommentID != "" {
				photoCommentIDs = append(photoCommentIDs, n.CommentID)
			}
		}
		if n.Kind == kindNoteComment {
			noteIDs = append(noteIDs, n.PostID)
			noteCommentIDs = append(noteCommentIDs, n.CommentID)
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

	if t.topics, err = a.notificationTopics(r, unique(topicIDs)); err != nil {
		return targets{}, err
	}

	if t.photos, err = a.notificationPhotos(r, token, unique(photoIDs)); err != nil {
		return targets{}, err
	}

	if t.notes, err = a.notificationNotes(r, token, unique(noteIDs)); err != nil {
		return targets{}, err
	}

	if t.quotes, err = a.notificationQuotes(r, unique(messageIDs), unique(photoCommentIDs), unique(noteCommentIDs)); err != nil {
		return targets{}, err
	}

	return t, nil
}

// notificationQuotes reads what the messages of topics and the comments
// of photos and notes a page of notifications names say, each kind in one
// request.
func (a *api) notificationQuotes(r *http.Request, messageIDs, photoCommentIDs, noteCommentIDs []string) (map[string]string, error) {
	quotes := make(map[string]string, len(messageIDs)+len(photoCommentIDs)+len(noteCommentIDs))

	if len(noteCommentIDs) > 0 {
		comments, err := a.notes.CommentsByID(r.Context(), noteCommentIDs)
		if err != nil {
			return nil, err
		}
		for _, c := range comments {
			quotes[c.ID] = c.Text
		}
	}

	if len(messageIDs) > 0 {
		messages, err := a.topics.MessagesByID(r.Context(), messageIDs)
		if err != nil {
			return nil, err
		}
		for _, m := range messages {
			quotes[m.ID] = m.Text
		}
	}

	if len(photoCommentIDs) > 0 {
		comments, err := a.albums.PhotoCommentsByID(r.Context(), photoCommentIDs)
		if err != nil {
			return nil, err
		}
		for _, c := range comments {
			quotes[c.ID] = c.Text
		}
	}

	return quotes, nil
}

// notificationPhotos reads the photos a page of notifications names, as
// notificationNotes reads the notes a page of notifications names, as
// notificationTopics reads the topics. They are the reader's own.
func (a *api) notificationNotes(r *http.Request, token string, ids []string) (map[string]model.Note, error) {
	notes := make(map[string]model.Note, len(ids))

	for _, id := range ids {
		note, err := a.notes.Get(r.Context(), token, id)
		if failure.From(err).Kind == failure.NotFound {
			continue
		}
		if err != nil {
			return nil, err
		}

		notes[id] = note
	}

	return notes, nil
}

// notificationTopics reads the topics. They are the reader's own.
func (a *api) notificationPhotos(r *http.Request, token string, ids []string) (map[string]model.AlbumPhoto, error) {
	photos := make(map[string]model.AlbumPhoto, len(ids))

	for _, id := range ids {
		photo, _, err := a.albums.Photo(r.Context(), token, id)
		if failure.From(err).Kind == failure.NotFound {
			continue
		}
		if err != nil {
			return nil, err
		}

		photos[id] = photo
	}

	return photos, nil
}

// notificationTopics reads the topics a page of notifications names, one
// request each: there are few. A deleted one is left out.
func (a *api) notificationTopics(r *http.Request, ids []string) (map[string]model.Topic, error) {
	topics := make(map[string]model.Topic, len(ids))

	for _, id := range ids {
		topic, err := a.topics.Get(r.Context(), "", id)
		if failure.From(err).Kind == failure.NotFound {
			continue
		}
		if err != nil {
			return nil, err
		}

		topics[id] = topic
	}

	return topics, nil
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

	if namesTopic(n.Kind) {
		topic, ok := t.topics[n.PostID]
		if !ok {
			return notificationJSON{}, false
		}
		item.Topic = &notificationTopicJSON{ID: topic.ID, Title: topic.Title}
		item.Comment = quoted(n, t)

		return item, true
	}

	if namesPhoto(n.Kind) {
		photo, ok := t.photos[n.PostID]
		if !ok {
			return notificationJSON{}, false
		}
		shown := toPhotoJSON(photo.AuthorID, photo.ID)
		item.Photo = &shown
		item.Comment = quoted(n, t)

		return item, true
	}

	if n.Kind == kindNoteComment {
		note, ok := t.notes[n.PostID]
		// A comment that is gone leaves nothing to tell about.
		if _, said := t.quotes[n.CommentID]; !ok || !said {
			return notificationJSON{}, false
		}
		item.Note = &notificationNoteJSON{ID: note.ID, Title: note.Title}
		item.Comment = quoted(n, t)

		return item, true
	}

	if n.PostID != "" && namesPost(n.Kind) {
		p, ok := t.posts[n.PostID]
		if !ok {
			return notificationJSON{}, false
		}
		item.Post = &notificationPostJSON{ID: p.ID, CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339), Text: p.Text}
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

// quoted is the message of a topic or the comment of a photo a
// notification names, with what it says; nil for none, or when it says
// nothing (photos only).
func quoted(n model.Notification, t targets) *notificationCommentJSON {
	text, ok := t.quotes[n.CommentID]
	if !ok || text == "" {
		return nil
	}

	return &notificationCommentJSON{ID: n.CommentID, Text: text}
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
