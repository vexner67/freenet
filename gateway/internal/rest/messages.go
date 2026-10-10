package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type messageRequest struct {
	Text string `json:"text"`
	// Photos are IDs from POST /api/v1/me/photos.
	Photos []string `json:"photos"`
	// Post is a post shared into the dialog ("Поделиться" "в переписку").
	Post string `json:"post"`
	// Forward are messages of the caller's dialogs to carry on ("Переслать").
	Forward []string `json:"forward"`
}

// messageJSON: Out is the caller's own message, Read means its recipient
// has read it. The caller may delete their message from both sides until
// DeletableUntil and edit it until EditableUntil, "" for the peer's.
// Deleted: its sender took it back, it says nothing ("Сообщение удалено").
type messageJSON struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// TextCut: text is only the beginning of what the message says, as
	// the list of dialogs carries it.
	TextCut        bool        `json:"textCut,omitempty"`
	Photos         []photoJSON `json:"photos"`
	CreatedAt      string      `json:"createdAt"`
	Out            bool        `json:"out"`
	Read           bool        `json:"read"`
	DeletableUntil string      `json:"deletableUntil"`
	EditableUntil  string      `json:"editableUntil"`
	// EditedAt is null for a message never edited.
	EditedAt *string `json:"editedAt"`
	// Post is a post shared in it, "" for none; Repost shows it, as inside
	// a repost, except in the list of dialogs.
	Post   string      `json:"post"`
	Repost *repostJSON `json:"repost,omitempty"`
	// ForwardedCount is how many messages it carries on; Forwarded shows
	// them, except in the list of dialogs and in a search, without those
	// whose authors' pages are gone.
	ForwardedCount int             `json:"forwardedCount"`
	Forwarded      []forwardedJSON `json:"forwarded,omitempty"`
	Deleted        bool            `json:"deleted"`

	// carried is what Forwarded is made of (showAttached).
	carried []model.ForwardedMessage
}

// forwardedJSON is a message carried on from another dialog, as its
// author wrote it then.
type forwardedJSON struct {
	Author    authorJSON  `json:"author"`
	Text      string      `json:"text"`
	Photos    []photoJSON `json:"photos"`
	CreatedAt string      `json:"createdAt"`
	Post      string      `json:"post"`
	Repost    *repostJSON `json:"repost,omitempty"`
}

type dialogJSON struct {
	Peer        authorJSON  `json:"peer"`
	LastMessage messageJSON `json:"lastMessage"`
	Unread      bool        `json:"unread"`
}

type dialogPageResponse struct {
	Dialogs    []dialogJSON `json:"dialogs"`
	NextCursor string       `json:"nextCursor"`
}

type messagePageResponse struct {
	Peer         authorJSON    `json:"peer"`
	Messages     []messageJSON `json:"messages"`
	NextCursor   string        `json:"nextCursor"`
	PeerReadUpTo string        `json:"peerReadUpTo"`
	// CanWrite: the peer's privacy and blacklists let the caller write;
	// told on the first page of a dialog, not on older pages or polls.
	CanWrite bool `json:"canWrite"`
}

// toMessageJSON: a message is the caller's when the peer did not send it.
func toMessageJSON(m model.Message, peerID string) messageJSON {
	j := messageJSON{
		ID:        m.ID,
		Text:      m.Text,
		TextCut:   m.TextCut,
		Photos:    toPhotosJSON(m.SenderID, m.PhotoIDs),
		CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		Out:       m.SenderID != peerID,
		Read:      m.Read,
		Post:      m.PostID,
		EditedAt:  optionalTime(m.EditedAt),

		ForwardedCount: len(m.Forwarded),
		Deleted:        !m.DeletedAt.IsZero(),
		carried:        m.Forwarded,
	}

	if j.Out && !j.Deleted {
		j.DeletableUntil = timeString(m.DeletableUntil)
		j.EditableUntil = timeString(m.EditableUntil)
	}

	return j
}

// listDialogs is the caller's dialogs with who is on the other side,
// looked up in one request; a peer who deleted the account is left out.
func (a *api) listDialogs(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.messages.Dialogs(r.Context(), token, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, len(page.Dialogs))
	for i, d := range page.Dialogs {
		ids[i] = d.PeerID
	}

	peers, err := a.people(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := dialogPageResponse{
		Dialogs:    make([]dialogJSON, 0, len(page.Dialogs)),
		NextCursor: page.NextCursor,
	}

	for _, d := range page.Dialogs {
		peer, ok := peers[d.PeerID]
		if !ok {
			continue
		}

		resp.Dialogs = append(resp.Dialogs, dialogJSON{
			Peer:        toAuthorJSON(peer),
			LastMessage: toMessageJSON(d.LastMessage, d.PeerID),
			Unread:      d.Unread,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// countUnreadDialogs is for the counter next to "Мои Сообщения".
func (a *api) countUnreadDialogs(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	count, err := a.messages.CountUnread(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, unreadCountResponse{Count: count})
}

// listMessages is the dialog with a user: back in time with ?cursor=,
// what came after a message with ?after= for polling, or the part around
// a message with ?around=, as a found message opens.
func (a *api) listMessages(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	peer, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	at := model.MessagesAt{Cursor: query.Get("cursor"), After: query.Get("after"), Around: query.Get("around")}

	page, err := a.messages.History(r.Context(), token, peer.ID, at, limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := messagePageResponse{
		Peer:         toAuthorJSON(authorOf(peer.ID, peer.AvatarID, peer.Profile)),
		Messages:     make([]messageJSON, 0, len(page.Messages)),
		NextCursor:   page.NextCursor,
		PeerReadUpTo: page.PeerReadUpTo,
	}

	for _, m := range page.Messages {
		resp.Messages = append(resp.Messages, toMessageJSON(m, peer.ID))
	}

	if err = a.showAttached(r, resp.Messages); err != nil {
		a.writeError(w, r, err)
		return
	}

	if at.Cursor == "" && at.After == "" {
		if resp.CanWrite, err = a.canMessage(r, token, peer.ID); err != nil {
			a.writeError(w, r, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// getMessage is a message of the dialog with a user, whole: the dialog
// carries the beginnings of long texts.
func (a *api) getMessage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	peer, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	m, err := a.messages.Whole(r.Context(), token, peer.ID, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown := []messageJSON{toMessageJSON(m, peer.ID)}
	if err = a.showAttached(r, shown); err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, shown[0])
}

// canMessage asks the user service, which keeps privacy and blacklists,
// whether the caller may write to peerID, telling it what only the message
// service knows: whether the peer wrote first, so may be answered.
func (a *api) canMessage(r *http.Request, token, peerID string) (bool, error) {
	wroteFirst, err := a.messages.HasMessagesFrom(r.Context(), token, peerID)
	if err != nil {
		return false, err
	}

	return a.users.CanMessage(r.Context(), token, peerID, wroteFirst)
}

// sendMessage needs a filled-in profile, as posting does: the recipient
// sees the sender's name and page. The recipient is checked here, since
// the message service does not know users.
func (a *api) sendMessage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req messageRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !me.ProfileCompleted {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "fill in your profile before writing messages"))
		return
	}

	recipient, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	allowed, err := a.canMessage(r, token, recipient.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !allowed {
		a.writeError(w, r, failure.New(failure.PermissionDenied, "you cannot write to this user"))
		return
	}

	if err = a.checkOwnPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.checkSharedPost(r, token, req.Post); err != nil {
		a.writeError(w, r, err)
		return
	}

	draft := model.MessageDraft{Text: req.Text, PhotoIDs: req.Photos, PostID: req.Post, ForwardIDs: req.Forward}

	m, err := a.messages.Send(r.Context(), token, recipient.ID, draft)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	j := []messageJSON{toMessageJSON(m, recipient.ID)}
	if err = a.showAttached(r, j); err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, j[0])
}

// checkSharedPost refuses a post to share that is not there.
func (a *api) checkSharedPost(r *http.Request, token, postID string) error {
	if postID == "" {
		return nil
	}

	_, err := a.posts.Get(r.Context(), token, postID)
	if failure.From(err).Kind == failure.NotFound || failure.From(err).Kind == failure.InvalidArgument {
		return failure.New(failure.InvalidArgument, "post not found")
	}

	return err
}

// showAttached fills in what messages show besides their own words: the
// posts shared in them and the messages they carry on, with their authors
// and the posts shared in those. A forwarded message whose author's page
// is gone is left out, as what they wrote is everywhere.
func (a *api) showAttached(r *http.Request, messages []messageJSON) error {
	var posts, people []string
	for _, m := range messages {
		if m.Post != "" {
			posts = append(posts, m.Post)
		}

		for _, f := range m.carried {
			people = append(people, f.SenderID)
			if f.PostID != "" {
				posts = append(posts, f.PostID)
			}
		}
	}

	shown, err := a.repostsByID(r, posts)
	if err != nil {
		return err
	}

	authors, err := a.people(r, people)
	if err != nil {
		return err
	}

	for i := range messages {
		m := &messages[i]
		if m.Post != "" {
			m.Repost = shown[m.Post]
		}

		for _, f := range m.carried {
			author, ok := authors[f.SenderID]
			if !ok {
				continue
			}

			fj := forwardedJSON{
				Author:    toAuthorJSON(author),
				Text:      f.Text,
				Photos:    toPhotosJSON(f.SenderID, f.PhotoIDs),
				CreatedAt: f.CreatedAt.UTC().Format(time.RFC3339),
				Post:      f.PostID,
			}
			if f.PostID != "" {
				fj.Repost = shown[f.PostID]
			}

			m.Forwarded = append(m.Forwarded, fj)
		}
	}

	return nil
}

// markDialogRead reads the dialog up to upTo, the newest message from the
// user that the page has shown.
func (a *api) markDialogRead(w http.ResponseWriter, r *http.Request) {
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

	peer, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.messages.MarkRead(r.Context(), token, peer.ID, req.UpTo); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// deleteMessage takes the caller's message back from both sides of the
// dialog, where it stays saying nothing, or, with ?for=me, deletes any message of their dialog for them
// alone ("Удалить у себя"); the message service decides whether they may.
func (a *api) deleteMessage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	remove := a.messages.Delete

	switch r.URL.Query().Get("for") {
	case "":
	case "me":
		remove = a.messages.Hide
	default:
		a.writeError(w, r, failure.New(failure.InvalidArgument, "for is \"me\" or nothing"))
		return
	}

	if err = remove(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// editMessage changes what the caller's message says; the message
// service decides whether they still may.
func (a *api) editMessage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	m, err := a.messages.Edit(r.Context(), token, r.PathValue("id"), req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// The caller sent it: the peer is its recipient.
	j := []messageJSON{toMessageJSON(m, m.RecipientID)}
	if err = a.showAttached(r, j); err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, j[0])
}

type foundMessageJSON struct {
	Peer    authorJSON  `json:"peer"`
	Message messageJSON `json:"message"`
}

type foundMessagesResponse struct {
	Messages   []foundMessageJSON `json:"messages"`
	NextCursor string             `json:"nextCursor"`
}

// searchMessages finds the caller's messages that say ?q=, newest first,
// each with whom it was written to or by: in the dialog with ?peer=, a
// nickname, or in all their dialogs. Messages of peers whose pages are
// gone are left out.
func (a *api) searchMessages(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	var peerID string
	if nickname := query.Get("peer"); nickname != "" {
		peer, err := a.users.GetProfile(r.Context(), "", nickname)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		peerID = peer.ID
	}

	found, err := a.messages.Search(r.Context(), token, peerID, query.Get("q"), query.Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// Who is on the other side of each message.
	other := func(m model.Message) string {
		if m.SenderID == me.ID {
			return m.RecipientID
		}

		return m.SenderID
	}

	ids := make([]string, 0, len(found.Messages))
	for _, m := range found.Messages {
		ids = append(ids, other(m))
	}

	peers, err := a.people(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := foundMessagesResponse{Messages: make([]foundMessageJSON, 0, len(found.Messages)), NextCursor: found.NextCursor}
	for _, m := range found.Messages {
		peer, ok := peers[other(m)]
		if !ok {
			continue
		}

		resp.Messages = append(resp.Messages, foundMessageJSON{Peer: toAuthorJSON(peer), Message: toMessageJSON(m, other(m))})
	}

	writeJSON(w, http.StatusOK, resp)
}
