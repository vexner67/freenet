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

func (h *PostHandler) PostponePost(ctx context.Context, req *postv1.PostponePostRequest) (*postv1.PostponePostResponse, error) {
	authorID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	wallID := authorID
	if req.GetWallId() != "" {
		if wallID, err = uuid.Parse(req.GetWallId()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid wall id")
		}
	}

	text, photos, err := fromProtoTextPhotos(req.GetText(), req.GetPhotoIds())
	if err != nil {
		return nil, err
	}

	content, err := post.NewContent(text, photos)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	mentions, err := parseMentions(req.GetMentionedIds())
	if err != nil {
		return nil, err
	}

	if !req.GetPublishAt().IsValid() {
		return nil, status.Error(codes.InvalidArgument, "invalid publish time")
	}

	p, err := h.postponedService.Postpone(ctx, post.PostponedDraft{
		WallID:    wallID,
		AuthorID:  authorID,
		Content:   content,
		FromWall:  req.GetFromWall(),
		Signed:    req.GetSigned(),
		NSFW:      req.GetNsfw(),
		Mentions:  mentions,
		PublishAt: req.GetPublishAt().AsTime(),
	})
	if err != nil {
		return nil, err
	}

	return &postv1.PostponePostResponse{Postponed: toProtoPostponed(p)}, nil
}

func (h *PostHandler) ListPostponed(ctx context.Context, req *postv1.ListPostponedRequest) (*postv1.ListPostponedResponse, error) {
	viewer, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	wallID, err := uuid.Parse(req.GetWallId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid wall id")
	}

	found, err := h.postponedService.List(ctx, viewer, wallID)
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListPostponedResponse{Postponed: make([]*postv1.PostponedPost, 0, len(found))}
	for _, p := range found {
		pp := toProtoPostponed(p)
		pp.Text, pp.TextCut = p.Content().Text().Excerpt(int(req.GetTextLimit()))
		resp.Postponed = append(resp.Postponed, pp)
	}

	return resp, nil
}

func (h *PostHandler) GetPostponed(ctx context.Context, req *postv1.GetPostponedRequest) (*postv1.GetPostponedResponse, error) {
	viewer, id, err := callerAndPostponed(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	p, err := h.postponedService.Get(ctx, viewer, id)
	if err != nil {
		return nil, err
	}

	return &postv1.GetPostponedResponse{Postponed: toProtoPostponed(p)}, nil
}

func (h *PostHandler) EditPostponed(ctx context.Context, req *postv1.EditPostponedRequest) (*postv1.EditPostponedResponse, error) {
	requesterID, id, err := callerAndPostponed(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	text, err := post.NewText(req.GetText())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	mentions, err := parseMentions(req.GetMentionedIds())
	if err != nil {
		return nil, err
	}

	if !req.GetPublishAt().IsValid() {
		return nil, status.Error(codes.InvalidArgument, "invalid publish time")
	}

	p, err := h.postponedService.Edit(ctx, requesterID, id, post.PostponedEdit{
		Text: text, Mentions: mentions, NSFW: req.GetNsfw(), PublishAt: req.GetPublishAt().AsTime(),
	})
	if err != nil {
		return nil, err
	}

	return &postv1.EditPostponedResponse{Postponed: toProtoPostponed(p)}, nil
}

func (h *PostHandler) PublishPostponed(ctx context.Context, req *postv1.PublishPostponedRequest) (*postv1.PublishPostponedResponse, error) {
	requesterID, id, err := callerAndPostponed(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	p, err := h.postponedService.Publish(ctx, requesterID, id)
	if err != nil {
		return nil, err
	}

	return &postv1.PublishPostponedResponse{Post: toProtoPost(p, false, requesterID)}, nil
}

func (h *PostHandler) DeletePostponed(ctx context.Context, req *postv1.DeletePostponedRequest) (*emptypb.Empty, error) {
	requesterID, id, err := callerAndPostponed(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.postponedService.Delete(ctx, requesterID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func callerAndPostponed(ctx context.Context, rawID string) (uuid.UUID, uuid.UUID, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, err := uuid.Parse(rawID)
	if err != nil {
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, "invalid postponed post id")
	}

	return userID, id, nil
}

func toProtoPostponed(p *post.Postponed) *postv1.PostponedPost {
	mentioned := make([]string, len(p.Mentions()))
	for i, id := range p.Mentions() {
		mentioned[i] = id.String()
	}

	return &postv1.PostponedPost{
		Id:           p.ID().String(),
		WallId:       p.WallID().String(),
		AuthorId:     p.AuthorID().String(),
		Text:         p.Content().Text().String(),
		PhotoIds:     photoStrings(p.Content().Photos()),
		FromWall:     p.FromWall(),
		Signed:       p.Signed(),
		Nsfw:         p.NSFW(),
		MentionedIds: mentioned,
		PublishAt:    timestamppb.New(p.PublishAt()),
		CreatedAt:    timestamppb.New(p.CreatedAt()),
	}
}
