package rest

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// editMessageRequest names what changes, not what is left: an edit from
// another window, which shows the message as it was, undoes nothing. A
// message of a chat says Text; BaseText is the text the edit started
// from.
type editMessageRequest struct {
	Text     string  `json:"text"`
	BaseText *string `json:"baseText"`
	// Sealed is what a message of a dialog says from now on, sealed by
	// the browser: base64 in JSON.
	Sealed []byte `json:"sealed"`
	// BaseSealed is what it said when the edit started, "" for nothing;
	// with it a message that says something else by now is not edited
	// (409).
	BaseSealed   []byte   `json:"baseSealed"`
	AddPhotos    []string `json:"addPhotos"`
	RemovePhotos []string `json:"removePhotos"`
}

// messageRequest is a message into a dialog or a chat. Into a dialog it
// says nothing the server can read: its words and those of what it
// forwards are Sealed by the sender's browser (base64 in JSON); into a
// chat, Text.
type messageRequest struct {
	Text   string `json:"text"`
	Sealed []byte `json:"sealed"`
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
	// Sticker is a sticker of a pack among the caller's stickers, alone.
	Sticker string `json:"sticker"`
	// Documents are IDs of the caller's documents the message attaches.
	Documents []string `json:"documents"`
	// ReplyTo is a message of the same dialog or chat it answers
	// ("Ответить"); the words it quotes of a dialog's go in Sealed.
	ReplyTo string `json:"replyTo"`
	// Price is what the caller agreed a message costs, the recipient's
	// messagePrice: a message to someone who asks votes for it is paid.
	Price int64 `json:"price"`
}

// messageJSON: Out is the caller's own message, Read means its recipient
// has read it. The caller may delete their message from both sides until
// DeletableUntil and edit it until EditableUntil, "" for the peer's.
// Deleted: its sender took it back, it says nothing ("Сообщение удалено").
// What it says is Sealed, base64: the browsers of the two open it.
type messageJSON struct {
	ID             string      `json:"id"`
	Sealed         []byte      `json:"sealed"`
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
	// Sticker is the sticker the message is, "" for none.
	Sticker string `json:"sticker"`
	// DocumentCount is how many documents it attaches; Documents shows
	// those that are still there, except in the list of dialogs.
	DocumentCount int            `json:"documentCount"`
	Documents     []documentJSON `json:"documents,omitempty"`
	// ReplyTo is what it shows of the message it answers; absent for none.
	ReplyTo *replyJSON `json:"replyTo,omitempty"`
	// Reactions are those to it, the oldest first.
	Reactions []reactionJSON `json:"reactions"`

	// carried is what Forwarded is made of, documentIDs what Documents is,
	// quote what ReplyTo is (showAttached).
	carried     []model.ForwardedMessage
	documentIDs []string
	quote       *model.Quote
}

// replyJSON is what an answer shows of the message it answers: who wrote
// it and, in short, what it is. Author is null, and the rest says
// nothing, when the message is gone for good or its author's page is
// gone. Text is a chat's beginning: a message of a dialog carries the
// words it quotes sealed itself.
type replyJSON struct {
	ID             string      `json:"id"`
	Author         *authorJSON `json:"author"`
	Text           string      `json:"text"`
	TextCut        bool        `json:"textCut"`
	PhotoCount     int         `json:"photoCount"`
	ForwardedCount int         `json:"forwardedCount"`
	DocumentCount  int         `json:"documentCount"`
	Sticker        string      `json:"sticker"`
	Post           bool        `json:"post"`
	AlbumPhoto     bool        `json:"albumPhoto"`
	// Deleted: it was taken back and says nothing.
	Deleted bool `json:"deleted"`
}

// reactionJSON is one person's reaction: heart, like, laugh, wow, sad or
// fire.
type reactionJSON struct {
	UserID   string `json:"userId"`
	Reaction string `json:"reaction"`
}

func toReactionsJSON(reactions []model.Reaction) []reactionJSON {
	out := make([]reactionJSON, len(reactions))
	for i, r := range reactions {
		out[i] = reactionJSON{UserID: r.UserID, Reaction: r.Reaction}
	}

	return out
}

// attachment is what a message shows besides its words, as the message
// service tells it.
type attachment struct {
	PostID        string
	SharedPhotoID string
	Sticker       string
	Forwarded     []model.ForwardedMessage
	DocumentIDs   []string
	ReplyTo       *model.Quote
	Reactions     []model.Reaction
}

func toAttachedJSON(at attachment) attachedJSON {
	return attachedJSON{
		Post: at.PostID, AlbumPhoto: at.SharedPhotoID, Sticker: at.Sticker, ForwardedCount: len(at.Forwarded), carried: at.Forwarded,
		DocumentCount: len(at.DocumentIDs), documentIDs: at.DocumentIDs, quote: at.ReplyTo, Reactions: toReactionsJSON(at.Reactions),
	}
}

// toReplyJSON shows q written by author; nil author for one gone.
func toReplyJSON(q *model.Quote, author *authorJSON) *replyJSON {
	if author == nil || !q.Found {
		return &replyJSON{ID: q.ID}
	}

	return &replyJSON{
		ID: q.ID, Author: author, Text: q.Text, TextCut: q.TextCut, PhotoCount: q.PhotoCount, ForwardedCount: q.ForwardedCount,
		DocumentCount: q.DocumentCount, Sticker: q.Sticker, Post: q.Post, AlbumPhoto: q.SharedPhoto, Deleted: q.Deleted,
	}
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
// carried it on, one level deeper. Index is its place among all those
// carried, the left out too: a message of a dialog seals their words in
// that order.
type forwardedJSON struct {
	Index     int         `json:"index"`
	Depth     int         `json:"depth"`
	Author    authorJSON  `json:"author"`
	Text      string      `json:"text"`
	Photos    []photoJSON `json:"photos"`
	CreatedAt string      `json:"createdAt"`
	Post      string      `json:"post"`
	Repost    *repostJSON `json:"repost,omitempty"`

	AlbumPhoto  string           `json:"albumPhoto"`
	SharedPhoto *sharedPhotoJSON `json:"sharedPhoto,omitempty"`
	Sticker     string           `json:"sticker"`
	Documents   []documentJSON   `json:"documents,omitempty"`
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
	// PrimeOnly: the peer is written to only by people with Freenet Prime,
	// which the caller has not. MessagePrice: a message to the peer costs
	// so many votes, 0 for free.
	PrimeOnly    bool  `json:"primeOnly"`
	MessagePrice int64 `json:"messagePrice"`
}

// toMessageJSON: a message is the caller's when the peer did not send it.
func toMessageJSON(m model.Message, peerID string) messageJSON {
	j := messageJSON{
		ID:        m.ID,
		Sealed:    m.Sealed,
		Photos:    toPhotosJSON(m.SenderID, m.PhotoIDs),
		CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		Out:       m.SenderID != peerID,
		Read:      m.Read,
		EditedAt:  optionalTime(m.EditedAt),

		attachedJSON: toAttachedJSON(attachment{
			PostID: m.PostID, SharedPhotoID: m.SharedPhotoID, Sticker: m.Sticker, Forwarded: m.Forwarded, DocumentIDs: m.DocumentIDs,
			ReplyTo: m.ReplyTo, Reactions: m.Reactions,
		}),
		Deleted: !m.DeletedAt.IsZero(),
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

	// The caller's own token: a page open only to users of the site, or to
	// those its owner follows, tells the anonymous none of its Prime style.
	peer, err := a.users.GetProfile(r.Context(), token, r.PathValue("nickname"))
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
		Peer:         toAuthorJSON(peerAuthor(peer)),
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
		terms, _, err := a.messageTerms(r, token, peer.ID)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		resp.CanWrite, resp.PrimeOnly, resp.MessagePrice = terms.Allowed, terms.PrimeOnly, terms.Price
	}

	writeJSON(w, http.StatusOK, resp)
}

// messageTerms asks the user service, which keeps privacy and blacklists,
// whether and for how much the caller may write to peerID, telling it what
// only the message service knows: whether the peer wrote first, so may be
// answered (freely). It returns that too.
func (a *api) messageTerms(r *http.Request, token, peerID string) (model.MessageTerms, bool, error) {
	wroteFirst, err := a.messages.HasMessagesFrom(r.Context(), token, peerID)
	if err != nil {
		return model.MessageTerms{}, false, err
	}

	terms, err := a.users.CanMessage(r.Context(), token, peerID, wroteFirst)

	return terms, wroteFirst, err
}

// errCannotMessage and errPrimeOnly refuse a message to someone whose
// privacy or blacklist keeps the caller out.
var (
	errCannotMessage = failure.New(failure.PermissionDenied, "you cannot write to this user")
	errPrimeOnly     = failure.New(failure.PermissionDenied, "only people with freenet prime can write to this user")
)

func refusal(terms model.MessageTerms) error {
	if terms.PrimeOnly {
		return errPrimeOnly
	}

	return errCannotMessage
}

// refundMessage gives back the votes of a paid message the message service
// refused: one that may have been written all the same stays paid.
func (a *api) refundMessage(r *http.Request, token string, payment model.MessagePayment, refused error) {
	if payment.ID == "" {
		return
	}

	switch failure.From(refused).Kind {
	case failure.InvalidArgument, failure.PermissionDenied, failure.NotFound, failure.Conflict, failure.TooManyRequests:
	default:
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), releaseTimeout)
	defer cancel()

	if err := a.users.RefundMessage(ctx, token, payment.ID); err != nil {
		a.logger.WarnContext(r.Context(), "the votes of an unsent message kept", "error", err)
	}
}

// errPlainText refuses the plain words of a message of a dialog: its two
// browsers seal them.
var errPlainText = failure.New(failure.InvalidArgument, "messages are end-to-end encrypted")

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

	// The words of a dialog go sealed: plain ones are refused before the
	// photos are claimed.
	if req.Text != "" {
		a.writeError(w, r, errPlainText)
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

	terms, wroteFirst, err := a.messageTerms(r, token, recipient.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !terms.Allowed {
		a.writeError(w, r, refusal(terms))
		return
	}

	if terms.Price != req.Price {
		a.writeError(w, r, failure.New(failure.Conflict, "the price of messages changed"))
		return
	}

	draft, err := a.draft(r, token, me.ID, req)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// The message is paid before it is sent, and the votes go back when
	// the message service refuses it: the two live in different services.
	payment, err := a.users.PayMessage(r.Context(), token, recipient.ID, wroteFirst, req.Price)
	if err != nil {
		a.releasePhotos(r, token, req.Photos, err)
		a.writeError(w, r, err)
		return
	}

	m, err := a.messages.Send(r.Context(), token, recipient.ID, draft)
	if err != nil {
		a.refundMessage(r, token, payment, err)
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
func (a *api) draft(r *http.Request, token, meID string, req messageRequest) (model.MessageDraft, error) {
	if err := a.checkDocuments(r.Context(), meID, req.Documents); err != nil {
		return model.MessageDraft{}, err
	}

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

	if err := a.checkSticker(r, token, req.Sticker); err != nil {
		return model.MessageDraft{}, err
	}

	if err := a.claimPhotos(r, token, req.Photos); err != nil {
		return model.MessageDraft{}, err
	}

	return model.MessageDraft{
		Text: req.Text, Sealed: req.Sealed, PhotoIDs: req.Photos, PostID: req.Post, SharedPhotoID: req.AlbumPhoto, ForwardIDs: req.Forward,
		Sticker: req.Sticker, DocumentIDs: req.Documents, ReplyToID: req.ReplyTo,
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
	var posts, people, documents []string
	for _, m := range messages {
		if m.Post != "" {
			posts = append(posts, m.Post)
		}
		documents = append(documents, m.documentIDs...)

		if m.quote != nil && m.quote.Found {
			people = append(people, m.quote.SenderID)
		}

		for _, f := range m.carried {
			people = append(people, f.SenderID)
			documents = append(documents, f.DocumentIDs...)
			if f.PostID != "" {
				posts = append(posts, f.PostID)
			}
		}
	}

	docs, err := a.documentsOf(r.Context(), documents)
	if err != nil {
		return err
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
		m.Documents = documentsJSON(m.documentIDs, docs)

		if m.quote != nil {
			var shown *authorJSON
			if author, ok := authors[m.quote.SenderID]; ok {
				j := toAuthorJSON(author)
				shown = &j
			}

			m.ReplyTo = toReplyJSON(m.quote, shown)
		}

		// gone is the depth of a message left out: those under it go too.
		gone := -1
		for i, f := range m.carried {
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
				Index:     i,
				Depth:     f.Depth,
				Author:    toAuthorJSON(author),
				Text:      f.Text,
				Photos:    toPhotosJSON(f.SenderID, f.PhotoIDs),
				CreatedAt: f.CreatedAt.UTC().Format(time.RFC3339),
				Post:      f.PostID,

				AlbumPhoto:  f.SharedPhotoID,
				SharedPhoto: sharedPhoto(f.SharedPhotoID),
				Sticker:     f.Sticker,
				Documents:   documentsJSON(f.DocumentIDs, docs),
			}
			if f.PostID != "" {
				fj.Repost = shown[f.PostID]
			}

			m.Forwarded = append(m.Forwarded, fj)
		}
	}

	return nil
}

// reactionRequest is the reaction to put on a message: heart, like,
// laugh, wow, sad or fire.
type reactionRequest struct {
	Reaction string `json:"reaction"`
}

// reaction is what a PUT puts, "" for a DELETE, which takes the caller's
// away.
func reaction(w http.ResponseWriter, r *http.Request) (string, error) {
	if r.Method == http.MethodDelete {
		return "", nil
	}

	var req reactionRequest
	if err := decode(w, r, &req); err != nil {
		return "", err
	}

	if req.Reaction == "" {
		return "", failure.New(failure.InvalidArgument, "invalid reaction")
	}

	return req.Reaction, nil
}

// reactToMessage puts the caller's reaction on a message of their dialog
// or takes it away; both sides hear the message as it is now.
func (a *api) reactToMessage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	given, err := reaction(w, r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.messages.React(r.Context(), token, r.PathValue("id"), given); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

// dialogTyping tells the peer that the caller is writing to them
// ("печатает..."): only to someone they may write to, as a message.
func (a *api) dialogTyping(w http.ResponseWriter, r *http.Request) {
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

	terms, _, err := a.messageTerms(r, token, peer.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !terms.Allowed {
		a.writeError(w, r, refusal(terms))
		return
	}

	if err = a.messages.Typing(r.Context(), token, peer.ID); err != nil {
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

	if req.Text != "" || req.BaseText != nil {
		a.writeError(w, r, errPlainText)
		return
	}

	if err = a.claimPhotos(r, token, req.AddPhotos); err != nil {
		a.writeError(w, r, err)
		return
	}

	edit := model.MessageEdit{Sealed: req.Sealed, BaseSealed: req.BaseSealed, AddPhotoIDs: req.AddPhotos, RemovePhotoIDs: req.RemovePhotos}

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

// foundMessageJSON is a found message of a chat. Messages of dialogs are
// sealed: only the browsers of their two people can search them.
type foundMessageJSON struct {
	Chat        *foundChatJSON   `json:"chat"`
	ChatMessage *chatMessageJSON `json:"chatMessage"`
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

// searchMessages finds the caller's messages of chats that say ?q=,
// newest first, each with its chat: in the chat ?chat= or in all of
// theirs. Messages of people whose pages are gone are left out.
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

	if query.Get("peer") != "" {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "messages of dialogs are end-to-end encrypted"))
		return
	}

	page, err := a.chats.Search(r.Context(), token, query.Get("chat"), query.Get("q"), query.Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown, err := a.shownFound(r, token, me, page.Messages)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, foundMessagesResponse{Messages: shown, NextCursor: page.NextCursor})
}

// shownFound are the found messages with the people and the chats they
// name. A message may be found by what it carries on: the forwarded
// messages come with it.
func (a *api) shownFound(r *http.Request, token, me string, rows []model.ChatMessage) ([]foundMessageJSON, error) {
	var (
		ids   []string
		chats = make(map[string]*foundChatJSON)
	)

	for _, row := range rows {
		ids = append(ids, row.SenderID)

		if _, ok := chats[row.ChatID]; ok {
			continue
		}

		// The message service tells a chat by itself: one request a chat.
		view, err := a.chats.Get(r.Context(), token, row.ChatID)
		if err != nil {
			return nil, err
		}

		chats[row.ChatID] = &foundChatJSON{ID: view.Chat.ID, Title: view.Chat.Title}
	}

	people, err := a.people(r, ids)
	if err != nil {
		return nil, err
	}

	shown := make([]foundMessageJSON, 0, len(rows))
	attached := make([]*attachedJSON, 0, len(rows))

	for _, row := range rows {
		if j := toChatMessageJSON(row, me, people); j != nil {
			shown = append(shown, foundMessageJSON{Chat: chats[row.ChatID], ChatMessage: j})
			attached = append(attached, &j.attachedJSON)
		}
	}

	if err = a.showAttached(r, attached); err != nil {
		return nil, err
	}

	return shown, nil
}

// keyJSON is a public key messages are sealed with (X25519), base64; null
// for a user who has none yet.
type keyJSON struct {
	PublicKey []byte `json:"publicKey"`
}

// getMyMessageKey is the caller's own key, as the server has it: a
// browser that holds another one is not the one that set it up.
func (a *api) getMyMessageKey(w http.ResponseWriter, r *http.Request) {
	token, me, err := a.chatCaller(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	key, err := a.messages.Key(r.Context(), token, me)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, keyJSON{PublicKey: key})
}

// setMessageKey makes a key the caller's browser made the one their
// messages are sealed with from now on.
func (a *api) setMessageKey(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req keyJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.messages.SetKey(r.Context(), token, req.PublicKey); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getMessageKey is the key messages to a user are sealed with.
func (a *api) getMessageKey(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	user, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	key, err := a.messages.Key(r.Context(), token, user.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, keyJSON{PublicKey: key})
}

// peerAuthor is the other side of a dialog over its history: the names,
// the photo and the checkmark.
func peerAuthor(p model.PublicProfile) model.Author {
	a := authorOf(p.ID, p.AvatarID, p.Profile)
	a.Verified = p.Verified
	a.Prime = p.Prime
	a.PrimeStyle = p.PrimeStyle

	return a
}
