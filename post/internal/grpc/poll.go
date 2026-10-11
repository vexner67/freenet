package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PollService interface {
	Of(ctx context.Context, viewerID uuid.UUID, postIDs []uuid.UUID) ([]service.PollView, error)
	Vote(ctx context.Context, userID, postID uuid.UUID, places []int) (service.PollView, error)
	Retract(ctx context.Context, userID, postID uuid.UUID) (service.PollView, error)
	Voters(ctx context.Context, postID uuid.UUID, place int, cursor *service.LikerCursor, size int) (service.LikerPage, error)
}

// GetPolls gives the polls of posts, as the caller sees them.
func (h *PostHandler) GetPolls(ctx context.Context, req *postv1.GetPollsRequest) (*postv1.GetPollsResponse, error) {
	ids := make([]uuid.UUID, 0, len(req.GetPostIds()))
	for _, raw := range req.GetPostIds() {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid post id")
		}
		ids = append(ids, id)
	}

	views, err := h.pollService.Of(ctx, viewerID(ctx), ids)
	if err != nil {
		return nil, err
	}

	polls := make([]*postv1.Poll, 0, len(views))
	for _, v := range views {
		polls = append(polls, toProtoPoll(v))
	}

	return &postv1.GetPollsResponse{Polls: polls}, nil
}

func (h *PostHandler) VotePoll(ctx context.Context, req *postv1.VotePollRequest) (*postv1.VotePollResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	places := make([]int, len(req.GetOptions()))
	for i, place := range req.GetOptions() {
		places[i] = int(place)
	}

	view, err := h.pollService.Vote(ctx, userID, postID, places)
	if err != nil {
		return nil, err
	}

	return &postv1.VotePollResponse{Poll: toProtoPoll(view)}, nil
}

func (h *PostHandler) RetractPollVote(ctx context.Context, req *postv1.RetractPollVoteRequest) (*postv1.RetractPollVoteResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	view, err := h.pollService.Retract(ctx, userID, postID)
	if err != nil {
		return nil, err
	}

	return &postv1.RetractPollVoteResponse{Poll: toProtoPoll(view)}, nil
}

// ListPollVoters is openvk's "Список проголосовавших" of an option.
func (h *PostHandler) ListPollVoters(ctx context.Context, req *postv1.ListPollVotersRequest) (*postv1.ListPollVotersResponse, error) {
	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	cursor, err := parseLikerCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.pollService.Voters(ctx, postID, int(req.GetOption()), cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	ids, next, total := toProtoLikers(page)

	return &postv1.ListPollVotersResponse{UserIds: ids, NextCursor: next, Total: total}, nil
}
