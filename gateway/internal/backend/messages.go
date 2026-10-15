package backend

import (
	"context"

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
		RecipientId: recipientID, Text: d.Text, Sealed: d.Sealed, PhotoIds: d.PhotoIDs, PostId: d.PostID, PhotoId: d.SharedPhotoID, DocumentIds: d.DocumentIDs,
		ForwardIds: d.ForwardIDs, Sticker: d.Sticker, ReplyToId: d.ReplyToID,
	})
	if err != nil {
		return model.Message{}, errs.Wrap("m.api.SendMessage", fromStatus(err))
	}

	return toMessage(resp.GetMessage()), nil
}

func (m *Messages) Dialogs(ctx context.Context, accessToken, cursor string, limit int) (model.DialogPage, error) {
	resp, err := m.api.ListDialogs(withToken(ctx, accessToken), &messagev1.ListDialogsRequest{Cursor: cursor, Limit: toLimit(limit)})
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

// History is the part of the dialog with peerID that at names.
func (m *Messages) History(ctx context.Context, accessToken, peerID string, at model.MessagesAt, limit int) (model.MessagePage, error) {
	resp, err := m.api.ListMessages(withToken(ctx, accessToken), &messagev1.ListMessagesRequest{
		PeerId: peerID,
		Cursor: at.Cursor,
		After:  at.After,
		Around: at.Around,
		Limit:  toLimit(limit),
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

// Typing tells peerID that the caller is writing to them.
func (m *Messages) Typing(ctx context.Context, accessToken, peerID string) error {
	if _, err := m.api.SetTyping(withToken(ctx, accessToken), &messagev1.SetTypingRequest{PeerId: peerID}); err != nil {
		return errs.Wrap("m.api.SetTyping", fromStatus(err))
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
		Id: id, Text: e.Text, Sealed: e.Sealed, BaseSealed: e.BaseSealed, AddPhotoIds: e.AddPhotoIDs, RemovePhotoIds: e.RemovePhotoIDs,
	})
	if err != nil {
		return model.Message{}, errs.Wrap("m.api.EditMessage", fromStatus(err))
	}

	return toMessage(resp.GetMessage()), nil
}

// React puts the caller's reaction on a message of their dialog, or
// takes theirs away for "".
func (m *Messages) React(ctx context.Context, accessToken, id, reaction string) error {
	if _, err := m.api.ReactToMessage(withToken(ctx, accessToken), &messagev1.ReactToMessageRequest{MessageId: id, Reaction: reaction}); err != nil {
		return errs.Wrap("m.api.ReactToMessage", fromStatus(err))
	}

	return nil
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

		if t := resp.GetTyping(); t != nil {
			return model.MessageEvent{Typing: &model.Typing{UserID: t.GetUserId(), ChatID: t.GetChatId()}}, nil
		}

		if cr := resp.GetChatRead(); cr != nil {
			return model.MessageEvent{ChatRead: &model.ChatRead{ChatID: cr.GetChatId(), UpTo: cr.GetUpToId()}}, nil
		}

		msg := toMessage(resp.GetMessage())

		return model.MessageEvent{Message: &msg}, nil
	}, nil
}

// SetKey makes publicKey the one messages to and from the caller are
// sealed with.
func (m *Messages) SetKey(ctx context.Context, accessToken string, publicKey []byte) error {
	if _, err := m.api.SetKey(withToken(ctx, accessToken), &messagev1.SetKeyRequest{PublicKey: publicKey}); err != nil {
		return errs.Wrap("m.api.SetKey", fromStatus(err))
	}

	return nil
}

// Key is the key messages to userID are sealed with, nil for none yet.
func (m *Messages) Key(ctx context.Context, accessToken, userID string) ([]byte, error) {
	resp, err := m.api.GetKey(withToken(ctx, accessToken), &messagev1.GetKeyRequest{UserId: userID})
	if err != nil {
		return nil, errs.Wrap("m.api.GetKey", fromStatus(err))
	}

	return resp.GetPublicKey(), nil
}

// toForwarded are the messages another one carries on.
func toForwarded(carried []*messagev1.ForwardedMessage) []model.ForwardedMessage {
	forwarded := make([]model.ForwardedMessage, 0, len(carried))
	for _, f := range carried {
		forwarded = append(forwarded, model.ForwardedMessage{
			SenderID:  f.GetSenderId(),
			Text:      f.GetText(),
			PhotoIDs:  f.GetPhotoIds(),
			PostID:    f.GetPostId(),
			CreatedAt: f.GetCreatedAt().AsTime(),

			SharedPhotoID: f.GetPhotoId(),
			Depth:         int(f.GetDepth()),
			Sticker:       f.GetSticker(),
			DocumentIDs:   f.GetDocumentIds(),
		})
	}

	return forwarded
}

// toQuote is nil for a message that answers none.
func toQuote(pq *messagev1.Quote) *model.Quote {
	if pq == nil {
		return nil
	}

	return &model.Quote{
		ID: pq.GetId(), SenderID: pq.GetSenderId(), Text: pq.GetText(), TextCut: pq.GetTextCut(),
		PhotoCount: int(pq.GetPhotoCount()), ForwardedCount: int(pq.GetForwardedCount()), DocumentCount: int(pq.GetDocumentCount()),
		Sticker: pq.GetSticker(), Post: pq.GetPost(), SharedPhoto: pq.GetPhoto(), Deleted: pq.GetDeleted(), Found: pq.GetFound(),
	}
}

func toReactions(pr []*messagev1.Reaction) []model.Reaction {
	out := make([]model.Reaction, len(pr))
	for i, r := range pr {
		out[i] = model.Reaction{UserID: r.GetUserId(), Reaction: r.GetReaction()}
	}

	return out
}

func toMessage(pm *messagev1.Message) model.Message {
	return model.Message{
		ReplyTo:        toQuote(pm.GetReplyTo()),
		Reactions:      toReactions(pm.GetReactions()),
		Forwarded:      toForwarded(pm.GetForwarded()),
		DeletedAt:      fromTimestamp(pm.GetDeletedAt()),
		ID:             pm.GetId(),
		SenderID:       pm.GetSenderId(),
		RecipientID:    pm.GetRecipientId(),
		Sealed:         pm.GetSealed(),
		PhotoIDs:       pm.GetPhotoIds(),
		DocumentIDs:    pm.GetDocumentIds(),
		CreatedAt:      pm.GetCreatedAt().AsTime(),
		Read:           pm.GetRead(),
		DeletableUntil: fromTimestamp(pm.GetDeletableUntil()),
		PostID:         pm.GetPostId(),
		SharedPhotoID:  pm.GetPhotoId(),
		Sticker:        pm.GetSticker(),
		EditedAt:       fromTimestamp(pm.GetEditedAt()),
		EditableUntil:  fromTimestamp(pm.GetEditableUntil()),
	}
}
