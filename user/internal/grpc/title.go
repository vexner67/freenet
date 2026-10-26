package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type TitleService interface {
	Grant(ctx context.Context, adminID uuid.UUID, nickname user.Nickname, name user.TitleName, icon user.TitleIcon, now time.Time) (*user.Title, error)
	Revoke(ctx context.Context, adminID, id uuid.UUID) error
	UserTitles(ctx context.Context, nickname user.Nickname, now time.Time) ([]*user.Title, error)
}

func toProtoTitle(t *user.Title) *userv1.Title {
	return &userv1.Title{
		Id:        t.ID().String(),
		Name:      t.Name().String(),
		Icon:      t.Icon().String(),
		GrantedAt: timestamppb.New(t.GrantedAt()),
	}
}

func (h *UserHandler) GetUserTitles(ctx context.Context, req *userv1.GetUserTitlesRequest) (*userv1.GetUserTitlesResponse, error) {
	nickname, err := pageOwner(req.GetNickname())
	if err != nil {
		return nil, err
	}

	titles, err := h.titles.UserTitles(ctx, nickname, time.Now())
	if err != nil {
		return nil, err
	}

	out := make([]*userv1.Title, len(titles))
	for i, t := range titles {
		out[i] = toProtoTitle(t)
	}

	return &userv1.GetUserTitlesResponse{Titles: out}, nil
}

func (h *UserHandler) GrantTitle(ctx context.Context, req *userv1.GrantTitleRequest) (*userv1.GrantTitleResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	nickname, err := pageOwner(req.GetNickname())
	if err != nil {
		return nil, err
	}

	name, err := user.NewTitleName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	icon, err := user.NewTitleIcon(req.GetIcon())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	title, err := h.titles.Grant(ctx, adminID, nickname, name, icon, time.Now())
	if err != nil {
		return nil, err
	}

	return &userv1.GrantTitleResponse{Title: toProtoTitle(title)}, nil
}

func (h *UserHandler) RevokeTitle(ctx context.Context, req *userv1.RevokeTitleRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	// A title ID of a wrong form names no title.
	id, err := uuid.Parse(req.GetTitleId())
	if err != nil {
		return nil, user.ErrTitleNotFound
	}

	if err = h.titles.Revoke(ctx, adminID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
