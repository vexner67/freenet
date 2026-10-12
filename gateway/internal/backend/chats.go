package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"github.com/vexner67/freenet/pkg/errs"
)

const (
	chatStatePrefix  = "CHAT_MEMBER_STATE_"
	chatActionPrefix = "CHAT_ACTION_"
)

// Chats are the chats of several people ("беседы") of the message service.
type Chats struct {
	api messagev1.MessageServiceClient
}

func NewChats(api messagev1.MessageServiceClient) *Chats {
	return &Chats{
		api: api,
	}
}

// Create starts a chat of the caller with memberIDs; the caller has
// checked that each lets them.
func (c *Chats) Create(ctx context.Context, accessToken, title string, memberIDs []string) (model.Chat, error) {
	resp, err := c.api.CreateChat(withToken(ctx, accessToken), &messagev1.CreateChatRequest{Title: title, MemberIds: memberIDs})
	if err != nil {
		return model.Chat{}, errs.Wrap("c.api.CreateChat", fromStatus(err))
	}

	return toChat(resp.GetChat()), nil
}

func (c *Chats) Get(ctx context.Context, accessToken, id string) (model.ChatView, error) {
	resp, err := c.api.GetChat(withToken(ctx, accessToken), &messagev1.GetChatRequest{ChatId: id})
	if err != nil {
		return model.ChatView{}, errs.Wrap("c.api.GetChat", fromStatus(err))
	}

	view := model.ChatView{Chat: toChat(resp.GetChat()), Members: make([]model.ChatMember, 0, len(resp.GetMembers()))}
	for _, m := range resp.GetMembers() {
		view.Members = append(view.Members, model.ChatMember{UserID: m.GetUserId(), JoinedAt: fromTimestamp(m.GetJoinedAt())})
	}

	return view, nil
}

// List is a page of the caller's chats with the beginnings of their last
// messages.
func (c *Chats) List(ctx context.Context, accessToken, cursor string, limit int) (model.ChatPage, error) {
	resp, err := c.api.ListChats(withToken(ctx, accessToken), &messagev1.ListChatsRequest{Cursor: cursor, Limit: toLimit(limit), TextLimit: model.PreviewLength})
	if err != nil {
		return model.ChatPage{}, errs.Wrap("c.api.ListChats", fromStatus(err))
	}

	page := model.ChatPage{Chats: make([]model.Chat, 0, len(resp.GetChats())), NextCursor: resp.GetNextCursor()}
	for _, pc := range resp.GetChats() {
		page.Chats = append(page.Chats, toChat(pc))
	}

	return page, nil
}

func (c *Chats) Rename(ctx context.Context, accessToken, id, title string) error {
	_, err := c.api.RenameChat(withToken(ctx, accessToken), &messagev1.RenameChatRequest{ChatId: id, Title: title})
	if err != nil {
		return errs.Wrap("c.api.RenameChat", fromStatus(err))
	}

	return nil
}

// Invite adds userIDs to the chat; the caller has checked that each lets
// them.
func (c *Chats) Invite(ctx context.Context, accessToken, id string, userIDs []string) error {
	_, err := c.api.AddChatMembers(withToken(ctx, accessToken), &messagev1.AddChatMembersRequest{ChatId: id, UserIds: userIDs})
	if err != nil {
		return errs.Wrap("c.api.AddChatMembers", fromStatus(err))
	}

	return nil
}

func (c *Chats) Kick(ctx context.Context, accessToken, id, userID string) error {
	_, err := c.api.RemoveChatMember(withToken(ctx, accessToken), &messagev1.RemoveChatMemberRequest{ChatId: id, UserId: userID})
	if err != nil {
		return errs.Wrap("c.api.RemoveChatMember", fromStatus(err))
	}

	return nil
}

func (c *Chats) Leave(ctx context.Context, accessToken, id string) error {
	_, err := c.api.LeaveChat(withToken(ctx, accessToken), &messagev1.LeaveChatRequest{ChatId: id})
	if err != nil {
		return errs.Wrap("c.api.LeaveChat", fromStatus(err))
	}

	return nil
}

func (c *Chats) Return(ctx context.Context, accessToken, id string) error {
	_, err := c.api.ReturnToChat(withToken(ctx, accessToken), &messagev1.ReturnToChatRequest{ChatId: id})
	if err != nil {
		return errs.Wrap("c.api.ReturnToChat", fromStatus(err))
	}

	return nil
}

// Send writes d into the chat.
func (c *Chats) Send(ctx context.Context, accessToken, id string, d model.MessageDraft) (model.ChatMessage, error) {
	resp, err := c.api.SendChatMessage(withToken(ctx, accessToken), &messagev1.SendChatMessageRequest{
		ChatId: id, Text: d.Text, PhotoIds: d.PhotoIDs, PostId: d.PostID, PhotoId: d.SharedPhotoID, ForwardIds: d.ForwardIDs,
	})
	if err != nil {
		return model.ChatMessage{}, errs.Wrap("c.api.SendChatMessage", fromStatus(err))
	}

	return toChatMessage(resp.GetMessage()), nil
}

// History is the part of the chat that at names, with the beginnings of
// long texts.
func (c *Chats) History(ctx context.Context, accessToken, id string, at model.MessagesAt, limit int) (model.ChatMessagePage, error) {
	resp, err := c.api.ListChatMessages(withToken(ctx, accessToken), &messagev1.ListChatMessagesRequest{
		ChatId: id, Cursor: at.Cursor, After: at.After, Around: at.Around, Limit: toLimit(limit), TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.ChatMessagePage{}, errs.Wrap("c.api.ListChatMessages", fromStatus(err))
	}

	page := model.ChatMessagePage{Messages: make([]model.ChatMessage, 0, len(resp.GetMessages())), NextCursor: resp.GetNextCursor()}
	for _, pm := range resp.GetMessages() {
		page.Messages = append(page.Messages, toChatMessage(pm))
	}

	return page, nil
}

// Message is a message of a chat of the caller with its whole text.
func (c *Chats) Message(ctx context.Context, accessToken, id string) (model.ChatMessage, error) {
	resp, err := c.api.GetChatMessage(withToken(ctx, accessToken), &messagev1.GetChatMessageRequest{Id: id})
	if err != nil {
		return model.ChatMessage{}, errs.Wrap("c.api.GetChatMessage", fromStatus(err))
	}

	return toChatMessage(resp.GetMessage()), nil
}

// Edit changes what the caller's message says and the photos it shows.
func (c *Chats) Edit(ctx context.Context, accessToken, id string, e model.MessageEdit) (model.ChatMessage, error) {
	resp, err := c.api.EditChatMessage(withToken(ctx, accessToken), &messagev1.EditChatMessageRequest{
		Id: id, Text: e.Text, AddPhotoIds: e.AddPhotoIDs, RemovePhotoIds: e.RemovePhotoIDs,
	})
	if err != nil {
		return model.ChatMessage{}, errs.Wrap("c.api.EditChatMessage", fromStatus(err))
	}

	return toChatMessage(resp.GetMessage()), nil
}

// Search finds the messages of the caller's chats that say query, in the
// chat with chatID or, for "", in all of theirs.
func (c *Chats) Search(ctx context.Context, accessToken, chatID, query, cursor string, limit int) (model.FoundChatMessages, error) {
	resp, err := c.api.SearchChatMessages(withToken(ctx, accessToken), &messagev1.SearchChatMessagesRequest{
		ChatId: chatID, Query: query, Cursor: cursor, Limit: toLimit(limit), TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.FoundChatMessages{}, errs.Wrap("c.api.SearchChatMessages", fromStatus(err))
	}

	found := model.FoundChatMessages{Messages: make([]model.ChatMessage, 0, len(resp.GetMessages())), NextCursor: resp.GetNextCursor()}
	for _, pm := range resp.GetMessages() {
		found.Messages = append(found.Messages, toChatMessage(pm))
	}

	return found, nil
}

func (c *Chats) Delete(ctx context.Context, accessToken, id string) error {
	_, err := c.api.DeleteChatMessage(withToken(ctx, accessToken), &messagev1.DeleteChatMessageRequest{Id: id})
	if err != nil {
		return errs.Wrap("c.api.DeleteChatMessage", fromStatus(err))
	}

	return nil
}

func (c *Chats) MarkRead(ctx context.Context, accessToken, id, upTo string) error {
	_, err := c.api.MarkChatRead(withToken(ctx, accessToken), &messagev1.MarkChatReadRequest{ChatId: id, UpToId: upTo})
	if err != nil {
		return errs.Wrap("c.api.MarkChatRead", fromStatus(err))
	}

	return nil
}

func toChat(pc *messagev1.Chat) model.Chat {
	c := model.Chat{
		ID:          pc.GetId(),
		Title:       pc.GetTitle(),
		OwnerID:     pc.GetOwnerId(),
		CreatedAt:   fromTimestamp(pc.GetCreatedAt()),
		MemberCount: int(pc.GetMemberCount()),
		State:       enumName(pc.GetState().String(), chatStatePrefix),
		Unread:      pc.GetUnread(),
	}

	if pc.GetLastMessage() != nil {
		last := toChatMessage(pc.GetLastMessage())
		c.LastMessage = &last
	}

	return c
}

func toChatMessage(pm *messagev1.ChatMessage) model.ChatMessage {
	m := model.ChatMessage{
		ID:             pm.GetId(),
		ChatID:         pm.GetChatId(),
		SenderID:       pm.GetSenderId(),
		Text:           pm.GetText(),
		TextCut:        pm.GetTextCut(),
		PhotoIDs:       pm.GetPhotoIds(),
		TargetID:       pm.GetTargetId(),
		CreatedAt:      fromTimestamp(pm.GetCreatedAt()),
		EditedAt:       fromTimestamp(pm.GetEditedAt()),
		DeletedAt:      fromTimestamp(pm.GetDeletedAt()),
		DeletableUntil: fromTimestamp(pm.GetDeletableUntil()),
		EditableUntil:  fromTimestamp(pm.GetEditableUntil()),
		Unread:         pm.GetUnread(),
		PostID:         pm.GetPostId(),
		SharedPhotoID:  pm.GetPhotoId(),
		Forwarded:      toForwarded(pm.GetForwarded()),
	}

	if pm.GetAction() != messagev1.ChatAction_CHAT_ACTION_NONE {
		m.Action = enumName(pm.GetAction().String(), chatActionPrefix)
	}

	return m
}
