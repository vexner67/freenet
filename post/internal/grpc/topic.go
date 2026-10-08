package grpc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *PostHandler) CreateTopic(ctx context.Context, req *postv1.CreateTopicRequest) (*postv1.CreateTopicResponse, error) {
	authorID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	clubID, err := uuid.Parse(req.GetClubId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid club id")
	}

	title, err := post.NewTopicTitle(req.GetTitle())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	in := service.StartTopicInput{ClubID: clubID, AuthorID: authorID, Title: title, FromWall: req.GetFromWall()}

	// The first message is optional, as "Текст" of openvk's new topic.
	if strings.TrimSpace(req.GetText()) != "" || len(req.GetPhotoIds()) > 0 {
		content, err := commentContent(req.GetText(), req.GetPhotoIds())
		if err != nil {
			return nil, err
		}
		in.Content = &content
	}

	t, err := h.topicService.Start(ctx, in)
	if err != nil {
		return nil, err
	}

	return &postv1.CreateTopicResponse{Topic: toProtoTopic(t, nil, authorID)}, nil
}

func (h *PostHandler) GetTopic(ctx context.Context, req *postv1.GetTopicRequest) (*postv1.GetTopicResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid topic id")
	}

	t, err := h.topicService.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return &postv1.GetTopicResponse{Topic: toProtoTopic(t, nil, viewerID(ctx))}, nil
}

func (h *PostHandler) ListTopics(ctx context.Context, req *postv1.ListTopicsRequest) (*postv1.ListTopicsResponse, error) {
	clubID, err := uuid.Parse(req.GetClubId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid club id")
	}

	after, err := parseTopicCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.topicService.List(ctx, clubID, strings.TrimSpace(req.GetQuery()), after, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)
	resp := &postv1.ListTopicsResponse{
		Topics:     make([]*postv1.Topic, 0, len(page.Topics)),
		NextCursor: topicCursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a count of rows
	}
	for _, t := range page.Topics {
		resp.Topics = append(resp.Topics, toProtoTopic(t, page.Last[t.ID()], viewer))
	}

	return resp, nil
}

func (h *PostHandler) EditTopic(ctx context.Context, req *postv1.EditTopicRequest) (*postv1.EditTopicResponse, error) {
	requesterID, id, err := callerAndTopic(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	title, err := post.NewTopicTitle(req.GetTitle())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	t, err := h.topicService.Change(ctx, requesterID, id, post.TopicChanges{
		Title: title, Closed: req.GetClosed(), Pinned: req.GetPinned(), Restricted: req.GetRestricted(),
	})
	if err != nil {
		return nil, err
	}

	return &postv1.EditTopicResponse{Topic: toProtoTopic(t, nil, requesterID)}, nil
}

func (h *PostHandler) DeleteTopic(ctx context.Context, req *postv1.DeleteTopicRequest) (*emptypb.Empty, error) {
	requesterID, id, err := callerAndTopic(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.topicService.Delete(ctx, requesterID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) CreateTopicMessage(ctx context.Context, req *postv1.CreateTopicMessageRequest) (*postv1.CreateTopicMessageResponse, error) {
	authorID, topicID, err := callerAndTopic(ctx, req.GetTopicId())
	if err != nil {
		return nil, err
	}

	content, err := commentContent(req.GetText(), req.GetPhotoIds())
	if err != nil {
		return nil, err
	}

	m, err := h.topicService.Write(ctx, service.TopicMessageInput{
		TopicID: topicID, AuthorID: authorID, Content: content, FromWall: req.GetFromWall(),
	})
	if err != nil {
		return nil, err
	}

	return &postv1.CreateTopicMessageResponse{Message: toProtoTopicMessage(m, authorID)}, nil
}

func (h *PostHandler) ListTopicMessages(ctx context.Context, req *postv1.ListTopicMessagesRequest) (*postv1.ListTopicMessagesResponse, error) {
	topicID, err := uuid.Parse(req.GetTopicId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid topic id")
	}

	after, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.topicService.Messages(ctx, topicID, after, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)
	resp := &postv1.ListTopicMessagesResponse{
		Messages:   make([]*postv1.TopicMessage, 0, len(page.Messages)),
		NextCursor: cursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a counter from an int4 column
	}
	for _, m := range page.Messages {
		resp.Messages = append(resp.Messages, toProtoTopicMessage(m, viewer))
	}

	return resp, nil
}

func (h *PostHandler) DeleteTopicMessage(ctx context.Context, req *postv1.DeleteTopicMessageRequest) (*emptypb.Empty, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid message id")
	}

	if err = h.topicService.DeleteMessage(ctx, requesterID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func callerAndTopic(ctx context.Context, rawID string) (uuid.UUID, uuid.UUID, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, err := uuid.Parse(rawID)
	if err != nil {
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, "invalid topic id")
	}

	return userID, id, nil
}

// topicCursorString writes where a page of topics ends as
// "<pinned 0|1>_<updated_at in µs>_<id>"; "" for the last page.
func topicCursorString(c *service.TopicCursor) string {
	if c == nil {
		return ""
	}

	pinned := 0
	if c.Pinned {
		pinned = 1
	}

	return fmt.Sprintf("%d_%d_%s", pinned, c.UpdatedAt.UnixMicro(), c.ID)
}

func parseTopicCursor(s string) (*service.TopicCursor, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // no cursor: the first page
	}

	invalid := status.Error(codes.InvalidArgument, "invalid cursor")

	parts := strings.Split(s, "_")
	if len(parts) != 3 || (parts[0] != "0" && parts[0] != "1") {
		return nil, invalid
	}

	micros, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, invalid
	}

	id, err := uuid.Parse(parts[2])
	if err != nil {
		return nil, invalid
	}

	return &service.TopicCursor{Pinned: parts[0] == "1", UpdatedAt: time.UnixMicro(micros).UTC(), ID: id}, nil
}

// toProtoTopic tells viewerID whether they started t; last is its newest
// message, nil if none or not looked up.
func toProtoTopic(t *post.Topic, last *post.TopicMessage, viewerID uuid.UUID) *postv1.Topic {
	pt := &postv1.Topic{
		Id:           t.ID().String(),
		ClubId:       t.ClubID().String(),
		AuthorId:     t.AuthorID().String(),
		Title:        t.Title().String(),
		FromWall:     t.FromWall(),
		Closed:       t.Closed(),
		Pinned:       t.Pinned(),
		Restricted:   t.Restricted(),
		CreatedAt:    timestamppb.New(t.CreatedAt()),
		UpdatedAt:    timestamppb.New(t.UpdatedAt()),
		MessageCount: int32(t.Messages()), //nolint:gosec // a counter from an int4 column
		Mine:         viewerID != uuid.Nil && viewerID == t.AuthorID(),
	}
	if last != nil {
		pt.LastMessage = toProtoTopicMessage(last, viewerID)
	}

	return pt
}

func toProtoTopicMessage(m *post.TopicMessage, viewerID uuid.UUID) *postv1.TopicMessage {
	return &postv1.TopicMessage{
		Id:        m.ID().String(),
		TopicId:   m.TopicID().String(),
		AuthorId:  m.AuthorID().String(),
		Text:      m.Content().Text().String(),
		PhotoIds:  photoStrings(m.Content().Photos()),
		FromWall:  m.FromWall(),
		CreatedAt: timestamppb.New(m.CreatedAt()),
		Mine:      viewerID != uuid.Nil && m.BelongsTo(viewerID),
	}
}
