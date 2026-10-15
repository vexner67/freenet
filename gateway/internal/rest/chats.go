package rest

import (
	"net/http"
	"strings"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// maxChatInvited is how many people one request may name: a chat holds 50
// with the one who invites.
const maxChatInvited = 49

type (
	createChatRequest struct {
		Title string `json:"title"`
		// Members are nicknames, at least two.
		Members []string `json:"members"`
	}

	renameChatRequest struct {
		Title string `json:"title"`
	}

	pinChatMessageRequest struct {
		MessageID string `json:"messageId"`
	}

	addChatMembersRequest struct {
		Members []string `json:"members"`
	}
)

// chatJSON is a chat of several people as the caller has it. State is
// "active", "left" (may come back) or "kicked"; Members are told by the
// chat itself, not by lists, and to nobody who is not in it anymore.
type chatJSON struct {
	ID string `json:"id"`
	// Title is "" for a chat called by who is in it.
	Title       string           `json:"title"`
	OwnerID     string           `json:"ownerId"`
	CreatedAt   string           `json:"createdAt"`
	MemberCount int              `json:"memberCount"`
	State       string           `json:"state"`
	Unread      bool             `json:"unread"`
	LastMessage *chatMessageJSON `json:"lastMessage"`
	Members     []authorJSON     `json:"members,omitempty"`
	// Photo is null for a chat without one: it is shown by who is in it.
	Photo *avatarJSON `json:"photo"`
	// Pinned is the message pinned above the chat, told by the chat
	// itself; absent for none.
	Pinned *chatMessageJSON `json:"pinned,omitempty"`
}

// chatMessageJSON is what Author said in a chat or, with Action, what they
// did there: "create" and "title" (Text is the title), "invite" and "kick"
// (of Target), "leave", "return", "pin" and "unpin" (a message), "photo"
// and "photo_remove" (the photo of the chat). Out is the caller's own, which they may
// edit until EditableUntil and take back until DeletableUntil. It shows
// what a message of a dialog may: a shared post, a shared photo of an
// album and the messages it carries on, except in the list of dialogs.
type chatMessageJSON struct {
	ID             string      `json:"id"`
	ChatID         string      `json:"chatId"`
	Author         authorJSON  `json:"author"`
	Out            bool        `json:"out"`
	Text           string      `json:"text"`
	TextCut        bool        `json:"textCut,omitempty"`
	Photos         []photoJSON `json:"photos"`
	Action         string      `json:"action"`
	Target         *authorJSON `json:"target,omitempty"`
	CreatedAt      string      `json:"createdAt"`
	EditedAt       *string     `json:"editedAt"`
	Deleted        bool        `json:"deleted"`
	DeletableUntil string      `json:"deletableUntil"`
	EditableUntil  string      `json:"editableUntil"`
	// Unread: someone else's, which the caller has not read.
	Unread bool `json:"unread"`
	attachedJSON
}

type chatMessagePageResponse struct {
	Messages   []chatMessageJSON `json:"messages"`
	NextCursor string            `json:"nextCursor"`
}

// toChatMessageJSON is nil when the page of who wrote it, or of whom it
// tells, is gone.
func toChatMessageJSON(m model.ChatMessage, me string, people map[string]model.Author) *chatMessageJSON {
	author, ok := people[m.SenderID]
	if !ok {
		return nil
	}

	j := &chatMessageJSON{
		ID:        m.ID,
		ChatID:    m.ChatID,
		Author:    toAuthorJSON(author),
		Out:       m.SenderID == me,
		Text:      m.Text,
		TextCut:   m.TextCut,
		Photos:    toPhotosJSON(m.SenderID, m.PhotoIDs),
		Action:    m.Action,
		CreatedAt: timeString(m.CreatedAt),
		EditedAt:  optionalTime(m.EditedAt),
		Deleted:   !m.DeletedAt.IsZero(),
		Unread:    m.Unread,

		attachedJSON: toAttachedJSON(attachment{
			PostID: m.PostID, SharedPhotoID: m.SharedPhotoID, Sticker: m.Sticker, Forwarded: m.Forwarded, DocumentIDs: m.DocumentIDs,
			ReplyTo: m.ReplyTo, Reactions: m.Reactions,
		}),
	}

	if m.TargetID != "" {
		target, ok := people[m.TargetID]
		if !ok {
			return nil
		}

		shown := toAuthorJSON(target)
		j.Target = &shown
	}

	if j.Out && !j.Deleted {
		j.DeletableUntil = timeString(m.DeletableUntil)
		j.EditableUntil = timeString(m.EditableUntil)
	}

	return j
}

// chatPeople are the people the messages name: who wrote and about whom.
func chatPeople(messages []model.ChatMessage) []string {
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		ids = append(ids, m.SenderID)
		if m.TargetID != "" {
			ids = append(ids, m.TargetID)
		}
	}

	return ids
}

func toChatJSON(c model.Chat, me string, people map[string]model.Author) chatJSON {
	j := chatJSON{
		ID:          c.ID,
		Title:       c.Title,
		OwnerID:     c.OwnerID,
		CreatedAt:   timeString(c.CreatedAt),
		MemberCount: c.MemberCount,
		State:       c.State,
		Unread:      c.Unread,
		Photo:       toAvatarJSON(c.ID, c.PhotoID),
	}

	if c.LastMessage != nil {
		j.LastMessage = toChatMessageJSON(*c.LastMessage, me, people)
	}

	return j
}

// shownChat is the chat with the profiles of who is in it.
func (a *api) shownChat(r *http.Request, me string, view model.ChatView) (chatJSON, error) {
	ids := make([]string, 0, len(view.Members)+2)
	for _, m := range view.Members {
		ids = append(ids, m.UserID)
	}

	if view.Chat.LastMessage != nil {
		ids = append(ids, chatPeople([]model.ChatMessage{*view.Chat.LastMessage})...)
	}

	people, err := a.people(r, ids)
	if err != nil {
		return chatJSON{}, err
	}

	j := toChatJSON(view.Chat, me, people)

	for _, m := range view.Members {
		if p, ok := people[m.UserID]; ok {
			j.Members = append(j.Members, toAuthorJSON(p))
		}
	}

	if view.Pinned != nil {
		pinned, err := a.shownChatMessages(r, me, []model.ChatMessage{*view.Pinned})
		if err != nil {
			return chatJSON{}, err
		}

		if len(pinned) == 1 {
			j.Pinned = &pinned[0]
		}
	}

	return j, nil
}

// shownChatMessages are the messages with the profiles of the people
// they name and with what they share and carry on, without those of
// people whose pages are gone.
func (a *api) shownChatMessages(r *http.Request, me string, messages []model.ChatMessage) ([]chatMessageJSON, error) {
	people, err := a.people(r, chatPeople(messages))
	if err != nil {
		return nil, err
	}

	shown := make([]chatMessageJSON, 0, len(messages))
	for _, m := range messages {
		if j := toChatMessageJSON(m, me, people); j != nil {
			shown = append(shown, *j)
		}
	}

	attached := make([]*attachedJSON, len(shown))
	for i := range shown {
		attached[i] = &shown[i].attachedJSON
	}

	if err = a.showAttached(r, attached); err != nil {
		return nil, err
	}

	return shown, nil
}

// invitees are the IDs of the people named by nickname to add to a chat.
// The user service, which keeps privacy and blacklists, tells whether
// each lets the caller: the message service does not know users.
func (a *api) invitees(r *http.Request, token string, nicknames []string) ([]string, error) {
	if len(nicknames) > maxChatInvited {
		return nil, failure.New(failure.Conflict, "the chat is full")
	}

	found, err := a.users.ResolveInvites(r.Context(), token, nicknames)
	if err != nil {
		return nil, err
	}

	byNickname := make(map[string]model.Invitee, len(found))
	for _, i := range found {
		byNickname[strings.ToLower(i.Nickname)] = i
	}

	ids := make([]string, 0, len(found))

	for _, nickname := range nicknames {
		i, ok := byNickname[strings.ToLower(strings.TrimSpace(nickname))]
		if !ok {
			return nil, failure.New(failure.NotFound, "user not found")
		}

		if !i.Allowed {
			return nil, failure.New(failure.PermissionDenied, "you cannot add this user to a chat")
		}

		ids = append(ids, i.UserID)
	}

	return ids, nil
}

// createChat needs a filled-in profile, as writing messages does: the
// others see the creator's name and page.
func (a *api) createChat(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req createChatRequest
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

	ids, err := a.invitees(r, token, req.Members)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	chat, err := a.chats.Create(r.Context(), token, req.Title, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeChat(w, r, token, me.ID, chat.ID, http.StatusCreated)
}

// writeChat answers with the chat as it is now.
func (a *api) writeChat(w http.ResponseWriter, r *http.Request, token, me, id string, status int) {
	view, err := a.chats.Get(r.Context(), token, id)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown, err := a.shownChat(r, me, view)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, status, shown)
}

// chatCaller is the token and the ID of who asks about a chat.
func (a *api) chatCaller(r *http.Request) (string, string, error) {
	token, err := bearerToken(r)
	if err != nil {
		return "", "", err
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		return "", "", err
	}

	return token, me.ID, nil
}

func (a *api) getChat(w http.ResponseWriter, r *http.Request) {
	token, me, err := a.chatCaller(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeChat(w, r, token, me, r.PathValue("id"), http.StatusOK)
}

// changeChat does what one of the requests that change a chat asks and
// answers with the chat as it has become.
func (a *api) changeChat(w http.ResponseWriter, r *http.Request, change func(token, id string) error) {
	token, me, err := a.chatCaller(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	id := r.PathValue("id")

	if err = change(token, id); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeChat(w, r, token, me, id, http.StatusOK)
}

func (a *api) renameChat(w http.ResponseWriter, r *http.Request) {
	var req renameChatRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.changeChat(w, r, func(token, id string) error {
		return a.chats.Rename(r.Context(), token, id, req.Title)
	})
}

func (a *api) pinChatMessage(w http.ResponseWriter, r *http.Request) {
	var req pinChatMessageRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.changeChat(w, r, func(token, id string) error {
		return a.chats.Pin(r.Context(), token, id, req.MessageID)
	})
}

func (a *api) unpinChatMessage(w http.ResponseWriter, r *http.Request) {
	a.changeChat(w, r, func(token, id string) error {
		return a.chats.Unpin(r.Context(), token, id)
	})
}

// uploadChatPhoto uploads the photo of a chat under its ID, shows it and
// only then deletes the photo it showed, as a club does. Only someone in
// the chat may: they are asked first, so that nobody else stores files
// under its ID.
func (a *api) uploadChatPhoto(w http.ResponseWriter, r *http.Request) {
	a.changeChat(w, r, func(token, id string) error {
		shown, err := a.inChat(r, token, id)
		if err != nil {
			return err
		}

		image, err := readImage(w, r)
		if err != nil {
			return err
		}

		photoID, err := a.media.UploadChatAvatar(r.Context(), token, id, image)
		if err != nil {
			return err
		}

		if err = a.chats.SetPhoto(r.Context(), token, id, photoID); err != nil {
			a.cleanUpChatPhotos(r, token, id, shown)
			return err
		}

		a.cleanUpChatPhotos(r, token, id, photoID)

		return nil
	})
}

func (a *api) deleteChatPhoto(w http.ResponseWriter, r *http.Request) {
	a.changeChat(w, r, func(token, id string) error {
		if err := a.chats.SetPhoto(r.Context(), token, id, ""); err != nil {
			return err
		}

		a.cleanUpChatPhotos(r, token, id, "")

		return nil
	})
}

// inChat is the photo the chat shows, for someone in it now.
func (a *api) inChat(r *http.Request, token, id string) (string, error) {
	view, err := a.chats.Get(r.Context(), token, id)
	if err != nil {
		return "", err
	}

	if view.Chat.State != model.ChatStateActive {
		return "", failure.New(failure.Conflict, "you are not in this chat")
	}

	return view.Chat.PhotoID, nil
}

// cleanUpChatPhotos deletes photos the chat no longer shows. A failure
// only leaves unused files behind, so it is logged, not returned.
func (a *api) cleanUpChatPhotos(r *http.Request, token, chatID, keep string) {
	if err := a.media.DeleteChatAvatars(r.Context(), token, chatID, keep); err != nil {
		a.logger.WarnContext(r.Context(), "old chat photos left behind", "error", err)
	}
}

func (a *api) addChatMembers(w http.ResponseWriter, r *http.Request) {
	var req addChatMembersRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.changeChat(w, r, func(token, id string) error {
		ids, err := a.invitees(r, token, req.Members)
		if err != nil {
			return err
		}

		return a.chats.Invite(r.Context(), token, id, ids)
	})
}

// removeChatMember is the creator's "Исключить из беседы"; the message
// service decides whether the caller may.
func (a *api) removeChatMember(w http.ResponseWriter, r *http.Request) {
	a.changeChat(w, r, func(token, id string) error {
		member, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
		if err != nil {
			return err
		}

		return a.chats.Kick(r.Context(), token, id, member.ID)
	})
}

func (a *api) leaveChat(w http.ResponseWriter, r *http.Request) {
	a.changeChat(w, r, func(token, id string) error {
		return a.chats.Leave(r.Context(), token, id)
	})
}

func (a *api) returnToChat(w http.ResponseWriter, r *http.Request) {
	a.changeChat(w, r, func(token, id string) error {
		return a.chats.Return(r.Context(), token, id)
	})
}

// listChatMessages is the chat back in time with ?cursor=, what came
// after a message with ?after=, for polling, or the part around a message
// with ?around=, as a found message opens.
func (a *api) listChatMessages(w http.ResponseWriter, r *http.Request) {
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

	at := model.MessagesAt{Cursor: query.Get("cursor"), After: query.Get("after"), Around: query.Get("around")}

	page, err := a.chats.History(r.Context(), token, r.PathValue("id"), at, limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown, err := a.shownChatMessages(r, me, page.Messages)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, chatMessagePageResponse{Messages: shown, NextCursor: page.NextCursor})
}

// writeChatMessage answers with one message of a chat.
func (a *api) writeChatMessage(w http.ResponseWriter, r *http.Request, me string, m model.ChatMessage, status int) {
	shown, err := a.shownChatMessages(r, me, []model.ChatMessage{m})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if len(shown) == 0 {
		a.writeError(w, r, failure.New(failure.NotFound, "message not found"))
		return
	}

	writeJSON(w, status, shown[0])
}

// sendChatMessage checks the photos and what the message shares, as a
// dialog does; who may write, the message service knows.
func (a *api) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	token, me, err := a.chatCaller(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req messageRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if len(req.Sealed) > 0 {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "messages of chats are not encrypted"))
		return
	}

	draft, err := a.draft(r, token, me, req)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	m, err := a.chats.Send(r.Context(), token, r.PathValue("id"), draft)
	if err != nil {
		a.releasePhotos(r, token, req.Photos, err)
		a.writeError(w, r, err)
		return
	}

	a.writeChatMessage(w, r, me, m, http.StatusCreated)
}

// getChatMessage is a message whole: the chat carries the beginnings of
// long texts.
func (a *api) getChatMessage(w http.ResponseWriter, r *http.Request) {
	token, me, err := a.chatCaller(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	m, err := a.chats.Message(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeChatMessage(w, r, me, m, http.StatusOK)
}

// editChatMessage changes what the caller's message says and the photos
// it shows, as editMessage does.
func (a *api) editChatMessage(w http.ResponseWriter, r *http.Request) {
	token, me, err := a.chatCaller(r)
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

	m, err := a.chats.Edit(r.Context(), token, r.PathValue("id"), edit)
	if err != nil {
		a.releasePhotos(r, token, req.AddPhotos, err)
		a.writeError(w, r, err)
		return
	}

	a.writeChatMessage(w, r, me, m, http.StatusOK)
}

// deleteChatMessage takes the caller's message back from everyone: it
// stays in its place saying nothing.
func (a *api) deleteChatMessage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.chats.Delete(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// reactToChatMessage puts the caller's reaction on a message of a chat
// or takes it away; everyone in the chat hears the message as it is now.
func (a *api) reactToChatMessage(w http.ResponseWriter, r *http.Request) {
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

	if err = a.chats.React(r.Context(), token, r.PathValue("id"), given); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) markChatRead(w http.ResponseWriter, r *http.Request) {
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

	if err = a.chats.MarkRead(r.Context(), token, r.PathValue("id"), req.UpTo); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// chatTyping tells the others in the chat that the caller is writing
// there ("печатает...").
func (a *api) chatTyping(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.chats.Typing(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
