package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"github.com/vexner67/freenet/pkg/errs"
)

type Messages struct {
	api messagev1.MessageServiceClient
}

func NewMessages(api messagev1.MessageServiceClient) *Messages {
	return &Messages{
		api: api,
	}
}

// Send writes d to recipientID.
func (m *Messages) Send(ctx context.Context, accessToken, recipientID string, d model.MessageDraft) (model.Message, error) {
	resp, err := m.api.SendMessage(withToken(ctx, accessToken), &messagev1.SendMessageRequest{
		RecipientId: recipientID, Text: d.Text, PhotoIds: d.PhotoIDs, PostId: d.PostID, PhotoId: d.SharedPhotoID,
		ForwardIds: d.ForwardIDs,
	})
	if err != nil {
		return model.Message{}, errs.Wrap("m.api.SendMessage", fromStatus(err))
	}

	return toMessage(resp.GetMessage()), nil
}

func (m *Messages) Dialogs(ctx context.Context, accessToken, cursor string, limit int) (model.DialogPage, error) {
	resp, err := m.api.ListDialogs(withToken(ctx, accessToken), &messagev1.ListDialogsRequest{Cursor: cursor, Limit: toLimit(limit), TextLimit: model.PreviewLength})
	if err != nil {
		return model.DialogPage{}, errs.Wrap("m.api.ListDialogs", fromStatus(err))
	}

	page := model.DialogPage{
		Dialogs:    make([]model.Dialog, 0, len(resp.GetDialogs())),
		NextCursor: resp.GetNextCursor(),
	}

	for _, d := range resp.GetDialogs() {
		page.Dialogs = append(page.Dialogs, model.Dialog{
			PeerID:      d.GetPeerId(),
			LastMessage: toMessage(d.GetLastMessage()),
			Unread:      d.GetUnread(),
		})
	}

	return page, nil
}

func (m *Messages) CountUnread(ctx context.Context, accessToken string) (int, error) {
	resp, err := m.api.CountUnreadDialogs(withToken(ctx, accessToken), &messagev1.CountUnreadDialogsRequest{})
	if err != nil {
		return 0, errs.Wrap("m.api.CountUnreadDialogs", fromStatus(err))
	}

	return int(resp.GetCount()), nil
}

// Whole is message id of the dialog with peerID with its whole text and
// the whole texts of those it carries on.
func (m *Messages) Whole(ctx context.Context, accessToken, peerID, id string) (model.Message, error) {
	resp, err := m.api.ListMessages(withToken(ctx, accessToken), &messagev1.ListMessagesRequest{PeerId: peerID, Around: id, Limit: 1})
	if err != nil {
		return model.Message{}, errs.Wrap("m.api.ListMessages", fromStatus(err))
	}

	for _, pm := range resp.GetMessages() {
		if pm.GetId() == id {
			return toMessage(pm), nil
		}
	}

	return model.Message{}, failure.New(failure.NotFound, "message not found")
}

// History is the part of the dialog with peerID that at names, with the
// beginnings of long texts.
func (m *Messages) History(ctx context.Context, accessToken, peerID string, at model.MessagesAt, limit int) (model.MessagePage, error) {
	resp, err := m.api.ListMessages(withToken(ctx, accessToken), &messagev1.ListMessagesRequest{
		PeerId: peerID,
		Cursor: at.Cursor,
		After:  at.After,
		Around: at.Around,
		Limit:  toLimit(limit),

		TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.MessagePage{}, errs.Wrap("m.api.ListMessages", fromStatus(err))
	}

	page := model.MessagePage{
		Messages:     make([]model.Message, 0, len(resp.GetMessages())),
		NextCursor:   resp.GetNextCursor(),
		PeerReadUpTo: resp.GetPeerReadUpTo(),
	}

	for _, pm := range resp.GetMessages() {
		page.Messages = append(page.Messages, toMessage(pm))
	}

	return page, nil
}

func (m *Messages) MarkRead(ctx context.Context, accessToken, peerID, upTo string) error {
	_, err := m.api.MarkDialogRead(withToken(ctx, accessToken), &messagev1.MarkDialogReadRequest{PeerId: peerID, UpToId: upTo})
	if err != nil {
		return errs.Wrap("m.api.MarkDialogRead", fromStatus(err))
	}

	return nil
}

// Delete takes the caller's message back from both sides of the dialog.
func (m *Messages) Delete(ctx context.Context, accessToken, id string) error {
	_, err := m.api.DeleteMessage(withToken(ctx, accessToken), &messagev1.DeleteMessageRequest{Id: id})
	if err != nil {
		return errs.Wrap("m.api.DeleteMessage", fromStatus(err))
	}

	return nil
}

// Hide deletes a message of the caller's dialog for the caller alone.
func (m *Messages) Hide(ctx context.Context, accessToken, id string) error {
	_, err := m.api.HideMessage(withToken(ctx, accessToken), &messagev1.HideMessageRequest{Id: id})
	if err != nil {
		return errs.Wrap("m.api.HideMessage", fromStatus(err))
	}

	return nil
}

// DeleteMany deletes several messages at once, all or none: for both
// sides or for the caller alone.
func (m *Messages) DeleteMany(ctx context.Context, accessToken string, ids []string, forEveryone bool) error {
	_, err := m.api.DeleteMessages(withToken(ctx, accessToken), &messagev1.DeleteMessagesRequest{Ids: ids, ForEveryone: forEveryone})
	if err != nil {
		return errs.Wrap("m.api.DeleteMessages", fromStatus(err))
	}

	return nil
}

// Edit changes what the caller's message says and, when the edit
// replaces them, its photos.
func (m *Messages) Edit(ctx context.Context, accessToken, id string, e model.MessageEdit) (model.Message, error) {
	resp, err := m.api.EditMessage(withToken(ctx, accessToken), &messagev1.EditMessageRequest{
		Id: id, Text: e.Text, PhotoIds: e.PhotoIDs, ReplacePhotos: e.ReplacePhotos,
	})
	if err != nil {
		return model.Message{}, errs.Wrap("m.api.EditMessage", fromStatus(err))
	}

	return toMessage(resp.GetMessage()), nil
}

// Search finds the caller's messages that say query, in the dialog with
// peerID or, for "", in all of theirs.
func (m *Messages) Search(ctx context.Context, accessToken, peerID, query, cursor string, limit int) (model.FoundMessages, error) {
	resp, err := m.api.SearchMessages(withToken(ctx, accessToken), &messagev1.SearchMessagesRequest{
		PeerId: peerID, Query: query, Cursor: cursor, Limit: toLimit(limit), TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.FoundMessages{}, errs.Wrap("m.api.SearchMessages", fromStatus(err))
	}

	found := model.FoundMessages{Messages: make([]model.Message, 0, len(resp.GetMessages())), NextCursor: resp.GetNextCursor()}
	for _, pm := range resp.GetMessages() {
		found.Messages = append(found.Messages, toMessage(pm))
	}

	return found, nil
}

// HasMessagesFrom tells whether peerID has ever written to the caller.
func (m *Messages) HasMessagesFrom(ctx context.Context, accessToken, peerID string) (bool, error) {
	resp, err := m.api.HasMessagesFrom(withToken(ctx, accessToken), &messagev1.HasMessagesFromRequest{PeerId: peerID})
	if err != nil {
		return false, errs.Wrap("m.api.HasMessagesFrom", fromStatus(err))
	}

	return resp.GetHasMessages(), nil
}

// Subscribe streams the caller's message events until ctx ends, the token
// expires (Unauthenticated) or the caller falls behind (TooManyRequests).
func (m *Messages) Subscribe(ctx context.Context, accessToken string) (model.MessageEvents, error) {
	stream, err := m.api.Subscribe(withToken(ctx, accessToken), &messagev1.SubscribeRequest{})
	if err != nil {
		return nil, errs.Wrap("m.api.Subscribe", fromStatus(err))
	}

	return func() (model.MessageEvent, error) {
		resp, err := stream.Recv()
		if err != nil {
			return model.MessageEvent{}, errs.Wrap("stream.Recv", fromStatus(err))
		}

		if r := resp.GetRead(); r != nil {
			return model.MessageEvent{Read: &model.ReadMark{ReaderID: r.GetReaderId(), PeerID: r.GetPeerId(), UpTo: r.GetUpToId()}}, nil
		}

		if d := resp.GetDeleted(); d != nil {
			return model.MessageEvent{Deleted: &model.DeletedMessage{ID: d.GetId(), SenderID: d.GetSenderId(), RecipientID: d.GetRecipientId()}}, nil
		}

		if e := resp.GetEdited(); e != nil {
			edited := toMessage(e)

			return model.MessageEvent{Edited: &edited}, nil
		}

		if cm := resp.GetChatMessage(); cm != nil {
			msg := toChatMessage(cm)

			return model.MessageEvent{ChatMessage: &msg}, nil
		}

		if ce := resp.GetChatEdited(); ce != nil {
			edited := toChatMessage(ce)

			return model.MessageEvent{ChatEdited: &edited}, nil
		}

		if cr := resp.GetChatRead(); cr != nil {
			return model.MessageEvent{ChatRead: &model.ChatRead{ChatID: cr.GetChatId(), UpTo: cr.GetUpToId()}}, nil
		}

		msg := toMessage(resp.GetMessage())

		return model.MessageEvent{Message: &msg}, nil
	}, nil
}

func toMessage(pm *messagev1.Message) model.Message {
	forwarded := make([]model.ForwardedMessage, 0, len(pm.GetForwarded()))
	for _, f := range pm.GetForwarded() {
		forwarded = append(forwarded, model.ForwardedMessage{
			SenderID:  f.GetSenderId(),
			Text:      f.GetText(),
			PhotoIDs:  f.GetPhotoIds(),
			PostID:    f.GetPostId(),
			CreatedAt: f.GetCreatedAt().AsTime(),

			SharedPhotoID: f.GetPhotoId(),
		})
	}

	return model.Message{
		Forwarded:      forwarded,
		DeletedAt:      fromTimestamp(pm.GetDeletedAt()),
		ID:             pm.GetId(),
		SenderID:       pm.GetSenderId(),
		RecipientID:    pm.GetRecipientId(),
		Text:           pm.GetText(),
		TextCut:        pm.GetTextCut(),
		PhotoIDs:       pm.GetPhotoIds(),
		CreatedAt:      pm.GetCreatedAt().AsTime(),
		Read:           pm.GetRead(),
		DeletableUntil: fromTimestamp(pm.GetDeletableUntil()),
		PostID:         pm.GetPostId(),
		SharedPhotoID:  pm.GetPhotoId(),
		EditedAt:       fromTimestamp(pm.GetEditedAt()),
		EditableUntil:  fromTimestamp(pm.GetEditableUntil()),
	}
}
