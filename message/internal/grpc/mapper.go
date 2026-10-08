package grpc

import (
	"github.com/google/uuid"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoMessage(m *message.Message, read bool) *messagev1.Message {
	return &messagev1.Message{
		Id:          m.ID().String(),
		SenderId:    m.SenderID().String(),
		RecipientId: m.RecipientID().String(),
		Text:        m.Text().String(),
		CreatedAt:   timestamppb.New(m.CreatedAt()),
		Read:        read,
		PhotoIds:    photoStrings(m.Photos()),
	}
}

func photoStrings(photos message.Photos) []string {
	out := make([]string, len(photos))
	for i, id := range photos {
		out[i] = id.String()
	}

	return out
}

// fromProtoContent reads what a message to send says.
func fromProtoContent(req *messagev1.SendMessageRequest) (message.Content, error) {
	text, err := message.NewText(req.GetText())
	if err != nil {
		return message.Content{}, status.Error(codes.InvalidArgument, err.Error())
	}

	ids := make([]uuid.UUID, 0, len(req.GetPhotoIds()))
	for _, s := range req.GetPhotoIds() {
		id, err := uuid.Parse(s)
		if err != nil {
			return message.Content{}, status.Error(codes.InvalidArgument, message.ErrInvalidPhoto.Error())
		}
		ids = append(ids, id)
	}

	photos, err := message.NewPhotos(ids)
	if err != nil {
		return message.Content{}, status.Error(codes.InvalidArgument, err.Error())
	}

	content, err := message.NewContent(text, photos)
	if err != nil {
		return message.Content{}, status.Error(codes.InvalidArgument, err.Error())
	}

	return content, nil
}

func toProtoDialog(d message.Dialog, userID uuid.UUID) *messagev1.Dialog {
	return &messagev1.Dialog{
		PeerId:      d.PeerID.String(),
		LastMessage: toProtoMessage(d.LastMessage, d.Marks.IsRead(d.LastMessage, userID)),
		Unread:      d.HasUnread(),
	}
}

func toListMessagesResponse(page service.HistoryPage, userID uuid.UUID) *messagev1.ListMessagesResponse {
	resp := &messagev1.ListMessagesResponse{
		Messages:     make([]*messagev1.Message, 0, len(page.Messages)),
		NextCursor:   optionalID(page.NextCursor),
		PeerReadUpTo: optionalID(page.Marks.Theirs),
	}

	for _, m := range page.Messages {
		resp.Messages = append(resp.Messages, toProtoMessage(m, page.Marks.IsRead(m, userID)))
	}

	return resp
}

// optionalID is "" for uuid.Nil.
func optionalID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}

	return id.String()
}

// toProtoEvent: a new message is sent unread, as it was just written.
func toProtoEvent(e service.Event) *messagev1.SubscribeResponse {
	if e.Read != nil {
		return &messagev1.SubscribeResponse{Event: &messagev1.SubscribeResponse_Read{Read: &messagev1.ReadMark{
			ReaderId: e.Read.ReaderID.String(),
			PeerId:   e.Read.PeerID.String(),
			UpToId:   e.Read.UpTo.String(),
		}}}
	}

	return &messagev1.SubscribeResponse{Event: &messagev1.SubscribeResponse_Message{Message: toProtoMessage(e.Message, false)}}
}
