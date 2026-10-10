package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *PostHandler) SuggestPost(ctx context.Context, req *postv1.SuggestPostRequest) (*postv1.SuggestPostResponse, error) {
	authorID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	wallID, err := uuid.Parse(req.GetWallId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid wall id")
	}

	text, photos, err := fromProtoTextPhotos(req.GetText(), req.GetPhotoIds())
	if err != nil {
		return nil, err
	}

	content, err := post.NewContent(text, photos)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	s, err := h.suggestionService.Suggest(ctx, wallID, authorID, content)
	if err != nil {
		return nil, err
	}

	return &postv1.SuggestPostResponse{Suggestion: toProtoSuggestion(s)}, nil
}

func (h *PostHandler) ListSuggestions(ctx context.Context, req *postv1.ListSuggestionsRequest) (*postv1.ListSuggestionsResponse, error) {
	viewer, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	wallID, err := uuid.Parse(req.GetWallId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid wall id")
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.suggestionService.List(ctx, viewer, wallID, req.GetMine(), cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListSuggestionsResponse{
		Suggestions: make([]*postv1.Suggestion, 0, len(page.Suggestions)),
		Total:       int32(page.Total), //nolint:gosec // a count of rows
	}
	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}
	for _, s := range page.Suggestions {
		ps := toProtoSuggestion(s)
		ps.Text, ps.TextCut = s.Content().Text().Excerpt(int(req.GetTextLimit()))
		resp.Suggestions = append(resp.Suggestions, ps)
	}

	return resp, nil
}

func (h *PostHandler) GetSuggestion(ctx context.Context, req *postv1.GetSuggestionRequest) (*postv1.GetSuggestionResponse, error) {
	viewer, id, err := callerAndSuggestion(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	s, err := h.suggestionService.Get(ctx, viewer, id)
	if err != nil {
		return nil, err
	}

	return &postv1.GetSuggestionResponse{Suggestion: toProtoSuggestion(s)}, nil
}

func (h *PostHandler) PublishSuggestion(ctx context.Context, req *postv1.PublishSuggestionRequest) (*postv1.PublishSuggestionResponse, error) {
	requesterID, id, err := callerAndSuggestion(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	text, err := post.NewText(req.GetText())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	p, err := h.suggestionService.Publish(ctx, requesterID, id, text, req.GetSigned())
	if err != nil {
		return nil, err
	}

	return &postv1.PublishSuggestionResponse{Post: toProtoPost(p, false, requesterID)}, nil
}

func (h *PostHandler) DeleteSuggestion(ctx context.Context, req *postv1.DeleteSuggestionRequest) (*emptypb.Empty, error) {
	requesterID, id, err := callerAndSuggestion(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.suggestionService.Withdraw(ctx, requesterID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func callerAndSuggestion(ctx context.Context, rawID string) (uuid.UUID, uuid.UUID, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, err := uuid.Parse(rawID)
	if err != nil {
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, "invalid suggestion id")
	}

	return userID, id, nil
}

func toProtoSuggestion(s *post.Suggestion) *postv1.Suggestion {
	return &postv1.Suggestion{
		Id:        s.ID().String(),
		WallId:    s.WallID().String(),
		AuthorId:  s.AuthorID().String(),
		Text:      s.Content().Text().String(),
		PhotoIds:  photoStrings(s.Content().Photos()),
		CreatedAt: timestamppb.New(s.CreatedAt()),
	}
}
