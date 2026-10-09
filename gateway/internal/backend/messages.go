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

// Send writes text, photos and a shared post ("" for none) to recipientID.
func (m *Messages) Send(ctx context.Context, accessToken, recipientID, text string, photoIDs []string, postID string) (model.Message, error) {
	resp, err := m.api.SendMessage(withToken(ctx, accessToken), &messagev1.SendMessageRequest{
		RecipientId: recipientID, Text: text, PhotoIds: photoIDs, PostId: postID,
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

// History pages back with cursor, or returns the messages after after.
func (m *Messages) History(ctx context.Context, accessToken, peerID, cursor, after string, limit int) (model.MessagePage, error) {
	resp, err := m.api.ListMessages(withToken(ctx, accessToken), &messagev1.ListMessagesRequest{
		PeerId: peerID,
		Cursor: cursor,
		After:  after,
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

// Delete takes the caller's message back from both sides of the dialog.
func (m *Messages) Delete(ctx context.Context, accessToken, id string) error {
	_, err := m.api.DeleteMessage(withToken(ctx, accessToken), &messagev1.DeleteMessageRequest{Id: id})
	if err != nil {
		return errs.Wrap("m.api.DeleteMessage", fromStatus(err))
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

		msg := toMessage(resp.GetMessage())

		return model.MessageEvent{Message: &msg}, nil
	}, nil
}

func toMessage(pm *messagev1.Message) model.Message {
	return model.Message{
		ID:             pm.GetId(),
		SenderID:       pm.GetSenderId(),
		RecipientID:    pm.GetRecipientId(),
		Text:           pm.GetText(),
		PhotoIDs:       pm.GetPhotoIds(),
		CreatedAt:      pm.GetCreatedAt().AsTime(),
		Read:           pm.GetRead(),
		DeletableUntil: fromTimestamp(pm.GetDeletableUntil()),
		PostID:         pm.GetPostId(),
	}
}
