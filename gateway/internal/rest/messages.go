package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type messageRequest struct {
	Text string `json:"text"`
}

// messageJSON: Out is the caller's own message, Read means its recipient
// has read it.
type messageJSON struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
	Out       bool   `json:"out"`
	Read      bool   `json:"read"`
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
}

// toMessageJSON: a message is the caller's when the peer did not send it.
func toMessageJSON(m model.Message, peerID string) messageJSON {
	return messageJSON{
		ID:        m.ID,
		Text:      m.Text,
		CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		Out:       m.SenderID != peerID,
		Read:      m.Read,
	}
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

	peers, err := a.authors(r, ids)
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

	peer, err := a.users.GetProfile(r.Context(), r.PathValue("nickname"))
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

	writeJSON(w, http.StatusOK, resp)
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

	recipient, err := a.users.GetProfile(r.Context(), r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	m, err := a.messages.Send(r.Context(), token, recipient.ID, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toMessageJSON(m, recipient.ID))
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

	peer, err := a.users.GetProfile(r.Context(), r.PathValue("nickname"))
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
