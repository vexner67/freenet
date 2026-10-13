package rest

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// editMessageRequest names what changes, not what is left: an edit from
// another window, which shows the message as it was, undoes nothing.
type editMessageRequest struct {
	Text string `json:"text"`
	// BaseText is the text the edit started from; with it a message that
	// says something else by now is not edited (409).
	BaseText     *string  `json:"baseText"`
	AddPhotos    []string `json:"addPhotos"`
	RemovePhotos []string `json:"removePhotos"`
}

type messageRequest struct {
	Text string `json:"text"`
	// Photos are IDs from POST /api/v1/me/photos.
	Photos []string `json:"photos"`
	// Post is a post shared into the dialog ("Поделиться" "в переписку").
	Post string `json:"post"`
	// Forward are messages of the caller's dialogs and chats to carry on
	// ("Переслать").
	Forward []string `json:"forward"`
	// AlbumPhoto is a photo of an album shared into the dialog
	// ("Поделиться" under a photo, "в переписку"), with text alone.
	AlbumPhoto string `json:"albumPhoto"`
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
	attachedJSON
	Deleted bool `json:"deleted"`
}

// attachedJSON is what a message of a dialog or of a chat shows besides
// its own words.
type attachedJSON struct {
	// Post is a post shared in it, "" for none; Repost shows it, as inside
	// a repost, except in the list of dialogs.
	Post   string      `json:"post"`
	Repost *repostJSON `json:"repost,omitempty"`
	// AlbumPhoto is a photo of an album shared in it, "" for none;
	// SharedPhoto shows it, as inside a post that shares it, except in the
	// list of dialogs.
	AlbumPhoto  string           `json:"albumPhoto"`
	SharedPhoto *sharedPhotoJSON `json:"sharedPhoto,omitempty"`
	// ForwardedCount is how many messages it carries on; Forwarded shows
	// them, except in the list of dialogs, without those whose authors'
	// pages are gone.
	ForwardedCount int             `json:"forwardedCount"`
	Forwarded      []forwardedJSON `json:"forwarded,omitempty"`

	// carried is what Forwarded is made of (showAttached).
	carried []model.ForwardedMessage
}

func toAttachedJSON(postID, sharedPhotoID string, forwarded []model.ForwardedMessage) attachedJSON {
	return attachedJSON{Post: postID, AlbumPhoto: sharedPhotoID, ForwardedCount: len(forwarded), carried: forwarded}
}

// attachedTo are the attachments of the messages, to fill in in place.
func attachedTo(messages []messageJSON) []*attachedJSON {
	out := make([]*attachedJSON, len(messages))
	for i := range messages {
		out[i] = &messages[i].attachedJSON
	}

	return out
}

// forwardedJSON is a message carried on from a dialog or a chat, as its
// author wrote it then. Depth nests them: a message follows the one that
// carried it on, one level deeper.
type forwardedJSON struct {
	Depth     int         `json:"depth"`
	Author    authorJSON  `json:"author"`
	Text      string      `json:"text"`
	Photos    []photoJSON `json:"photos"`
	CreatedAt string      `json:"createdAt"`
	Post      string      `json:"post"`
	Repost    *repostJSON `json:"repost,omitempty"`

	AlbumPhoto  string           `json:"albumPhoto"`
	SharedPhoto *sharedPhotoJSON `json:"sharedPhoto,omitempty"`
}

// dialogJSON is a row of "Мои Сообщения": the dialog with Peer or, with
// Chat, a chat of several people, which carries its own last message.
type dialogJSON struct {
	Peer        *authorJSON  `json:"peer,omitempty"`
	LastMessage *messageJSON `json:"lastMessage,omitempty"`
	Chat        *chatJSON    `json:"chat,omitempty"`
	Unread      bool         `json:"unread"`
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
		EditedAt:  optionalTime(m.EditedAt),

		attachedJSON: toAttachedJSON(m.PostID, m.SharedPhotoID, m.Forwarded),
		Deleted:      !m.DeletedAt.IsZero(),
	}

	if j.Out && !j.Deleted {
		j.DeletableUntil = timeString(m.DeletableUntil)
		j.EditableUntil = timeString(m.EditableUntil)
	}

	return j
}

// conversation is a row of "Мои Сообщения": a dialog or a chat, placed
// by the ID of its last message.
type conversation struct {
	last   string
	dialog *model.Dialog
	chat   *model.Chat
}

// conversations are a page of the caller's dialogs and chats in one
// list, the most recent first. Both are read by the ID of the last
// message, which grows with time in both, so a page of each merges into a
// page of the two; the cursor is the last message of the last row.
func (a *api) conversations(r *http.Request, token, cursor string, limit int) ([]conversation, string, error) {
	if limit == 0 {
		limit = defaultPageSize
	}

	dialogs, err := a.messages.Dialogs(r.Context(), token, cursor, limit)
	if err != nil {
		return nil, "", err
	}

	chats, err := a.chats.List(r.Context(), token, cursor, limit)
	if err != nil {
		return nil, "", err
	}

	rows := make([]conversation, 0, len(dialogs.Dialogs)+len(chats.Chats))
	for i := range dialogs.Dialogs {
		rows = append(rows, conversation{last: dialogs.Dialogs[i].LastMessage.ID, dialog: &dialogs.Dialogs[i]})
	}

	for i := range chats.Chats {
		if last := chats.Chats[i].LastMessage; last != nil {
			rows = append(rows, conversation{last: last.ID, chat: &chats.Chats[i]})
		}
	}

	slices.SortFunc(rows, func(x, y conversation) int { return strings.Compare(y.last, x.last) })

	more := dialogs.NextCursor != "" || chats.NextCursor != "" || len(rows) > limit
	if len(rows) > limit {
		rows = rows[:limit]
	}

	if !more || len(rows) == 0 {
		return rows, "", nil
	}

	return rows, rows[len(rows)-1].last, nil
}

// listDialogs is the caller's dialogs and chats with who is on the other
// side, looked up in one request; a peer who deleted the account is left
// out.
func (a *api) listDialogs(w http.ResponseWriter, r *http.Request) {
	token, me, err := a.chatCaller(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	rows, next, err := a.conversations(r, token, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(rows))
	for _, c := range rows {
		if c.dialog != nil {
			ids = append(ids, c.dialog.PeerID)
			continue
		}

		ids = append(ids, chatPeople([]model.ChatMessage{*c.chat.LastMessage})...)
	}

	people, err := a.people(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := dialogPageResponse{
		Dialogs:    make([]dialogJSON, 0, len(rows)),
		NextCursor: next,
	}

	for _, c := range rows {
		if c.chat != nil {
			chat := toChatJSON(*c.chat, me, people)
			resp.Dialogs = append(resp.Dialogs, dialogJSON{Chat: &chat, Unread: c.chat.Unread})

			continue
		}

		peer, ok := people[c.dialog.PeerID]
		if !ok {
			continue
		}

		shown, last := toAuthorJSON(peer), toMessageJSON(c.dialog.LastMessage, c.dialog.PeerID)

		resp.Dialogs = append(resp.Dialogs, dialogJSON{Peer: &shown, LastMessage: &last, Unread: c.dialog.Unread})
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

	if err = a.showAttached(r, attachedTo(resp.Messages)); err != nil {
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
	if err = a.showAttached(r, attachedTo(shown)); err != nil {
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

	draft, err := a.draft(r, token, req)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	m, err := a.messages.Send(r.Context(), token, recipient.ID, draft)
	if err != nil {
		a.releasePhotos(r, token, req.Photos, err)
		a.writeError(w, r, err)
		return
	}

	j := []messageJSON{toMessageJSON(m, recipient.ID)}
	if err = a.showAttached(r, attachedTo(j)); err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, j[0])
}

// draft is the message of a dialog or of a chat the request asks to
// send, once what it shares is there for the caller and its photos are
// claimed: the message service knows neither posts nor photos. A caller
// whom the send is refused gives the photos back (releasePhotos).
func (a *api) draft(r *http.Request, token string, req messageRequest) (model.MessageDraft, error) {
	if err := a.checkSharedPost(r, token, req.Post); err != nil {
		return model.MessageDraft{}, err
	}

	// A photo the caller is not shown is not theirs to pass on.
	if req.AlbumPhoto != "" {
		if _, err := a.visiblePhotoByID(r, token, req.AlbumPhoto); err != nil {
			if kind := failure.From(err).Kind; kind == failure.NotFound || kind == failure.InvalidArgument || kind == failure.PermissionDenied {
				err = failure.New(failure.InvalidArgument, "photo not found")
			}

			return model.MessageDraft{}, err
		}
	}

	if err := a.claimPhotos(r, token, req.Photos); err != nil {
		return model.MessageDraft{}, err
	}

	return model.MessageDraft{
		Text: req.Text, PhotoIDs: req.Photos, PostID: req.Post, SharedPhotoID: req.AlbumPhoto, ForwardIDs: req.Forward,
	}, nil
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
// is gone is left out, as what they wrote is everywhere, with what it
// carried on.
func (a *api) showAttached(r *http.Request, messages []*attachedJSON) error {
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

	// A photo shared more than once on the page is read once.
	photos := make(map[string]*sharedPhotoJSON)
	sharedPhoto := func(id string) *sharedPhotoJSON {
		if id == "" {
			return nil
		}

		if _, ok := photos[id]; !ok {
			photos[id] = a.sharedPhoto(r, id)
		}

		return photos[id]
	}

	for _, m := range messages {
		if m.Post != "" {
			m.Repost = shown[m.Post]
		}
		m.SharedPhoto = sharedPhoto(m.AlbumPhoto)

		// gone is the depth of a message left out: those under it go too.
		gone := -1
		for _, f := range m.carried {
			if gone >= 0 && f.Depth > gone {
				continue
			}

			author, ok := authors[f.SenderID]
			if !ok {
				gone = f.Depth
				continue
			}

			gone = -1

			fj := forwardedJSON{
				Depth:     f.Depth,
				Author:    toAuthorJSON(author),
				Text:      f.Text,
				Photos:    toPhotosJSON(f.SenderID, f.PhotoIDs),
				CreatedAt: f.CreatedAt.UTC().Format(time.RFC3339),
				Post:      f.PostID,

				AlbumPhoto:  f.SharedPhotoID,
				SharedPhoto: sharedPhoto(f.SharedPhotoID),
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

// maxDeletedMessages is how many messages one request deletes, as the
// message service takes.
const maxDeletedMessages = 100

// deleteMessages deletes the messages named by ?id=, all or none: taken
// back from both sides or, with ?for=me, for the caller alone.
func (a *api) deleteMessages(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()
	forEveryone := true

	switch query.Get("for") {
	case "":
	case "me":
		forEveryone = false
	default:
		a.writeError(w, r, failure.New(failure.InvalidArgument, "for is \"me\" or nothing"))
		return
	}

	ids := query["id"]
	if len(ids) == 0 {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "no messages to delete"))
		return
	}

	if len(ids) > maxDeletedMessages {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "too many messages"))
		return
	}

	if err = a.messages.DeleteMany(r.Context(), token, ids, forEveryone); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// editMessage changes what the caller's message says and the photos it
// shows: uploads to add, which are claimed as when sending, and photos
// to remove; the message service decides whether they still may.
func (a *api) editMessage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req editMessageRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.claimPhotos(r, token, req.AddPhotos); err != nil {
		a.writeError(w, r, err)
		return
	}

	edit := model.MessageEdit{Text: req.Text, BaseText: req.BaseText, AddPhotoIDs: req.AddPhotos, RemovePhotoIDs: req.RemovePhotos}

	m, err := a.messages.Edit(r.Context(), token, r.PathValue("id"), edit)
	if err != nil {
		a.releasePhotos(r, token, req.AddPhotos, err)
		a.writeError(w, r, err)
		return
	}

	// The caller sent it: the peer is its recipient.
	j := []messageJSON{toMessageJSON(m, m.RecipientID)}
	if err = a.showAttached(r, attachedTo(j)); err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, j[0])
}

// foundMessageJSON is a found message of the dialog with Peer or, with
// Chat, of a chat of several people.
type foundMessageJSON struct {
	Peer        *authorJSON      `json:"peer,omitempty"`
	Message     *messageJSON     `json:"message,omitempty"`
	Chat        *foundChatJSON   `json:"chat,omitempty"`
	ChatMessage *chatMessageJSON `json:"chatMessage,omitempty"`
}

// foundChatJSON is the chat a found message is of; Title is "" for a
// chat called by who is in it.
type foundChatJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type foundMessagesResponse struct {
	Messages   []foundMessageJSON `json:"messages"`
	NextCursor string             `json:"nextCursor"`
}

// found is a message a search found, of a dialog or of a chat, placed by
// its ID.
type found struct {
	id      string
	message *model.Message
	chat    *model.ChatMessage
}

// foundIn are a page of the caller's messages that say query, newest
// first: of the dialog with peerID, of the chat chatID or, with neither,
// of all their dialogs and chats in one list. The IDs of both grow with
// time, so a page of each merges into a page of the two, as the list of
// dialogs does (conversations).
func (a *api) foundIn(r *http.Request, token, peerID, chatID, query, cursor string, limit int) ([]found, string, error) {
	if limit == 0 {
		limit = defaultPageSize
	}

	var (
		rows []found
		// floor is the oldest message of a page that has more below it:
		// nothing older is told yet, or the next page would skip what
		// that source still has between them.
		floor string
	)

	below := func(next string) {
		if next > floor {
			floor = next
		}
	}

	if chatID == "" {
		page, err := a.messages.Search(r.Context(), token, peerID, query, cursor, limit)
		if err != nil {
			return nil, "", err
		}

		for i := range page.Messages {
			rows = append(rows, found{id: page.Messages[i].ID, message: &page.Messages[i]})
		}

		below(page.NextCursor)
	}

	if peerID == "" {
		page, err := a.chats.Search(r.Context(), token, chatID, query, cursor, limit)
		if err != nil {
			return nil, "", err
		}

		for i := range page.Messages {
			rows = append(rows, found{id: page.Messages[i].ID, chat: &page.Messages[i]})
		}

		below(page.NextCursor)
	}

	slices.SortFunc(rows, func(x, y found) int { return strings.Compare(y.id, x.id) })

	more := floor != ""
	rows = slices.DeleteFunc(rows, func(f found) bool { return f.id < floor })

	if len(rows) > limit {
		rows, more = rows[:limit], true
	}

	if !more || len(rows) == 0 {
		return rows, "", nil
	}

	return rows, rows[len(rows)-1].id, nil
}

// searchMessages finds the caller's messages that say ?q=, newest first,
// each with whom it was written to or by, or with its chat: in the dialog
// with ?peer=, a nickname, in the chat ?chat=, or in all their dialogs and
// chats. Messages of people whose pages are gone are left out.
func (a *api) searchMessages(w http.ResponseWriter, r *http.Request) {
	token, me, err := a.chatCaller(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	chatID := query.Get("chat")

	var peerID string
	if nickname := query.Get("peer"); nickname != "" {
		if chatID != "" {
			a.writeError(w, r, failure.New(failure.InvalidArgument, "peer and chat cannot be used together"))
			return
		}

		peer, err := a.users.GetProfile(r.Context(), "", nickname)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		peerID = peer.ID
	}

	rows, next, err := a.foundIn(r, token, peerID, chatID, query.Get("q"), query.Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown, err := a.shownFound(r, token, me, rows)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, foundMessagesResponse{Messages: shown, NextCursor: next})
}

// shownFound are the found messages with the people and the chats they
// name. A message may be found by what it carries on: the forwarded
// messages come with it.
func (a *api) shownFound(r *http.Request, token, me string, rows []found) ([]foundMessageJSON, error) {
	// Who is on the other side of a message of a dialog.
	other := func(m *model.Message) string {
		if m.SenderID == me {
			return m.RecipientID
		}

		return m.SenderID
	}

	var (
		ids   []string
		chats = make(map[string]*foundChatJSON)
	)

	for _, row := range rows {
		if row.message != nil {
			ids = append(ids, other(row.message))
			continue
		}

		ids = append(ids, row.chat.SenderID)

		if _, ok := chats[row.chat.ChatID]; ok {
			continue
		}

		// The message service tells a chat by itself: one request a chat.
		view, err := a.chats.Get(r.Context(), token, row.chat.ChatID)
		if err != nil {
			return nil, err
		}

		chats[row.chat.ChatID] = &foundChatJSON{ID: view.Chat.ID, Title: view.Chat.Title}
	}

	people, err := a.people(r, ids)
	if err != nil {
		return nil, err
	}

	shown := make([]foundMessageJSON, 0, len(rows))
	attached := make([]*attachedJSON, 0, len(rows))

	for _, row := range rows {
		if row.chat != nil {
			if j := toChatMessageJSON(*row.chat, me, people); j != nil {
				shown = append(shown, foundMessageJSON{Chat: chats[row.chat.ChatID], ChatMessage: j})
				attached = append(attached, &j.attachedJSON)
			}

			continue
		}

		peer, ok := people[other(row.message)]
		if !ok {
			continue
		}

		with, j := toAuthorJSON(peer), toMessageJSON(*row.message, other(row.message))

		shown = append(shown, foundMessageJSON{Peer: &with, Message: &j})
		attached = append(attached, &j.attachedJSON)
	}

	if err = a.showAttached(r, attached); err != nil {
		return nil, err
	}

	return shown, nil
}
