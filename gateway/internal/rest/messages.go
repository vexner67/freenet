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
}

// messageJSON: Out is the caller's own message, Read means its recipient
// has read it. The caller may delete their message until DeletableUntil,
// "" for the peer's.
type messageJSON struct {
	ID             string      `json:"id"`
	Text           string      `json:"text"`
	Photos         []photoJSON `json:"photos"`
	CreatedAt      string      `json:"createdAt"`
	Out            bool        `json:"out"`
	Read           bool        `json:"read"`
	DeletableUntil string      `json:"deletableUntil"`
	// Post is a post shared in it, "" for none; Repost shows it, as inside
	// a repost, except in the list of dialogs.
	Post   string      `json:"post"`
	Repost *repostJSON `json:"repost,omitempty"`
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
		Photos:    toPhotosJSON(m.SenderID, m.PhotoIDs),
		CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		Out:       m.SenderID != peerID,
		Read:      m.Read,
		Post:      m.PostID,
	}

	if j.Out {
		j.DeletableUntil = timeString(m.DeletableUntil)
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

// listMessages is the dialog with a user: back in time with ?cursor=, or
// what came after a message with ?after= for polling.
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

	page, err := a.messages.History(r.Context(), token, peer.ID, query.Get("cursor"), query.Get("after"), limit)
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

	if err = a.showSharedPosts(r, resp.Messages); err != nil {
		a.writeError(w, r, err)
		return
	}

	if query.Get("cursor") == "" && query.Get("after") == "" {
		if resp.CanWrite, err = a.canMessage(r, token, peer.ID); err != nil {
			a.writeError(w, r, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, resp)
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

	m, err := a.messages.Send(r.Context(), token, recipient.ID, req.Text, req.Photos, req.Post)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	j := []messageJSON{toMessageJSON(m, recipient.ID)}
	if err = a.showSharedPosts(r, j); err != nil {
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

// showSharedPosts fills in the posts shared in messages.
func (a *api) showSharedPosts(r *http.Request, messages []messageJSON) error {
	var ids []string
	for _, m := range messages {
		if m.Post != "" {
			ids = append(ids, m.Post)
		}
	}

	shown, err := a.repostsByID(r, ids)
	if err != nil {
		return err
	}

	for i := range messages {
		if messages[i].Post != "" {
			messages[i].Repost = shown[messages[i].Post]
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
// dialog; the message service decides whether they may.
func (a *api) deleteMessage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.messages.Delete(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
