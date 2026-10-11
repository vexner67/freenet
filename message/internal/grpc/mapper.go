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
	pm := &messagev1.Message{
		Id:          m.ID().String(),
		SenderId:    m.SenderID().String(),
		RecipientId: m.RecipientID().String(),
		Text:        m.Text().String(),
		CreatedAt:   timestamppb.New(m.CreatedAt()),
		Read:        read,
		PhotoIds:    photoStrings(m.Photos()),
		PostId:      postString(m.Post()),
		PhotoId:     postString(m.SharedPhoto()),
		Forwarded:   toProtoForwarded(m.Forwarded()),
	}

	// A message taken back can be neither edited nor taken back again.
	if m.IsDeleted() {
		pm.DeletedAt = timestamppb.New(m.DeletedAt())

		return pm
	}

	pm.DeletableUntil = timestamppb.New(m.DeletableUntil())
	pm.EditableUntil = timestamppb.New(m.EditableUntil())

	if !m.EditedAt().IsZero() {
		pm.EditedAt = timestamppb.New(m.EditedAt())
	}

	return pm
}

func toProtoForwarded(forwarded message.Forwards) []*messagev1.ForwardedMessage {
	out := make([]*messagev1.ForwardedMessage, len(forwarded))
	for i, f := range forwarded {
		out[i] = &messagev1.ForwardedMessage{
			SenderId:  f.SenderID.String(),
			Text:      f.Text.String(),
			PhotoIds:  photoStrings(f.Photos),
			PostId:    postString(f.Post),
			PhotoId:   postString(f.SharedPhoto),
			CreatedAt: timestamppb.New(f.CreatedAt),
		}
	}

	return out
}

// postString is "" for no post or no shared photo, as the proto fields say.
func postString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}

	return id.String()
}

func photoStrings(photos message.Photos) []string {
	out := make([]string, len(photos))
	for i, id := range photos {
		out[i] = id.String()
	}

	return out
}

// fromProtoDraft reads the message to send.
func fromProtoDraft(req *messagev1.SendMessageRequest) (service.Draft, error) {
	text, err := message.NewText(req.GetText())
	if err != nil {
		return service.Draft{}, status.Error(codes.InvalidArgument, err.Error())
	}

	ids, err := parseIDs(req.GetPhotoIds(), message.ErrInvalidPhoto.Error())
	if err != nil {
		return service.Draft{}, err
	}

	photos, err := message.NewPhotos(ids)
	if err != nil {
		return service.Draft{}, status.Error(codes.InvalidArgument, err.Error())
	}

	var post uuid.UUID
	if req.GetPostId() != "" {
		if post, err = uuid.Parse(req.GetPostId()); err != nil {
			return service.Draft{}, status.Error(codes.InvalidArgument, "invalid post id")
		}
	}

	var sharedPhoto uuid.UUID
	if req.GetPhotoId() != "" {
		if sharedPhoto, err = uuid.Parse(req.GetPhotoId()); err != nil {
			return service.Draft{}, status.Error(codes.InvalidArgument, message.ErrInvalidPhoto.Error())
		}
	}

	if len(req.GetForwardIds()) > message.MaxForwarded {
		return service.Draft{}, status.Error(codes.InvalidArgument, message.ErrTooManyForwarded.Error())
	}

	forward, err := parseIDs(req.GetForwardIds(), "invalid forwarded message id")
	if err != nil {
		return service.Draft{}, err
	}

	return service.Draft{Text: text, Photos: photos, Post: post, SharedPhoto: sharedPhoto, Forward: forward}, nil
}

// fromProtoRevision reads what an edit makes of a message.
func fromProtoRevision(req *messagev1.EditMessageRequest) (message.Revision, error) {
	text, err := message.NewText(req.GetText())
	if err != nil {
		return message.Revision{}, status.Error(codes.InvalidArgument, err.Error())
	}

	rev := message.Revision{Text: text, ReplacePhotos: req.GetReplacePhotos()}
	if !rev.ReplacePhotos {
		return rev, nil
	}

	ids, err := parseIDs(req.GetPhotoIds(), message.ErrInvalidPhoto.Error())
	if err != nil {
		return message.Revision{}, err
	}

	if rev.Photos, err = message.NewPhotos(ids); err != nil {
		return message.Revision{}, status.Error(codes.InvalidArgument, err.Error())
	}

	return rev, nil
}

func parseIDs(raw []string, invalid string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, invalid)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// excerpted cuts what a message says and carries on to limit characters,
// as a list asks; 0 leaves it whole.
func excerpted(pm *messagev1.Message, m *message.Message, limit int32) *messagev1.Message {
	pm.Text, pm.TextCut = m.Text().Excerpt(int(limit))

	for i, f := range m.Forwarded() {
		text, cut := f.Text.Excerpt(int(limit))
		pm.Forwarded[i].Text = text
		pm.TextCut = pm.TextCut || cut
	}

	return pm
}

// found cuts a found message as excerpted does, each text that says query
// to where it says it: its own and those of the messages it carries on.
func found(pm *messagev1.Message, m *message.Message, query message.Query, limit int32) *messagev1.Message {
	pm.Text, pm.TextCut = m.Text().ExcerptAround(query.String(), int(limit))

	for i, f := range m.Forwarded() {
		text, cut := f.Text.ExcerptAround(query.String(), int(limit))
		pm.Forwarded[i].Text = text
		pm.TextCut = pm.TextCut || cut
	}

	return pm
}

// toProtoDialog cuts the text of the last message to textLimit.
func toProtoDialog(d message.Dialog, userID uuid.UUID, textLimit int32) *messagev1.Dialog {
	return &messagev1.Dialog{
		PeerId:      d.PeerID.String(),
		LastMessage: excerpted(toProtoMessage(d.LastMessage, d.Marks.IsRead(d.LastMessage, userID)), d.LastMessage, textLimit),
		Unread:      d.HasUnread(),
	}
}

// toListMessagesResponse cuts the texts of the page to textLimit.
func toListMessagesResponse(page service.HistoryPage, userID uuid.UUID, textLimit int32) *messagev1.ListMessagesResponse {
	resp := &messagev1.ListMessagesResponse{
		Messages:     make([]*messagev1.Message, 0, len(page.Messages)),
		NextCursor:   optionalID(page.NextCursor),
		PeerReadUpTo: optionalID(page.Marks.Theirs),
	}

	for _, m := range page.Messages {
		resp.Messages = append(resp.Messages, excerpted(toProtoMessage(m, page.Marks.IsRead(m, userID)), m, textLimit))
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
	if e.Chat != nil {
		return toProtoChatEvent(e.Chat)
	}

	if e.Deleted != nil {
		return &messagev1.SubscribeResponse{Event: &messagev1.SubscribeResponse_Deleted{Deleted: &messagev1.MessageDeleted{
			Id:          e.Deleted.ID.String(),
			SenderId:    e.Deleted.SenderID.String(),
			RecipientId: e.Deleted.RecipientID.String(),
		}}}
	}

	if e.Edited != nil {
		return &messagev1.SubscribeResponse{Event: &messagev1.SubscribeResponse_Edited{Edited: toProtoMessage(e.Edited, false)}}
	}

	if e.Read != nil {
		return &messagev1.SubscribeResponse{Event: &messagev1.SubscribeResponse_Read{Read: &messagev1.ReadMark{
			ReaderId: e.Read.ReaderID.String(),
			PeerId:   e.Read.PeerID.String(),
			UpToId:   e.Read.UpTo.String(),
		}}}
	}

	return &messagev1.SubscribeResponse{Event: &messagev1.SubscribeResponse_Message{Message: toProtoMessage(e.Message, false)}}
}
