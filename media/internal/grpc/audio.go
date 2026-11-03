package grpc

import (
	"context"

	"github.com/google/uuid"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"github.com/vexner67/freenet/media/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AudioService interface {
	Upload(ctx context.Context, ownerID uuid.UUID, data []byte) (uuid.UUID, error)
	Get(ctx context.Context, ownerID, audioID uuid.UUID) ([]byte, error)
	Delete(ctx context.Context, ownerID uuid.UUID, audioIDs []uuid.UUID) error
}

func (h *MediaHandler) UploadAudio(ctx context.Context, req *mediav1.UploadAudioRequest) (*mediav1.UploadAudioResponse, error) {
	ownerID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := h.audio.Upload(ctx, ownerID, req.GetData())
	if err != nil {
		return nil, err
	}

	return &mediav1.UploadAudioResponse{AudioId: id.String()}, nil
}

func (h *MediaHandler) GetAudio(ctx context.Context, req *mediav1.GetAudioRequest) (*mediav1.GetAudioResponse, error) {
	ownerID, err := parseID(req.GetOwnerId(), "owner id")
	if err != nil {
		return nil, err
	}

	audioID, err := parseID(req.GetAudioId(), "audio id")
	if err != nil {
		return nil, err
	}

	data, err := h.audio.Get(ctx, ownerID, audioID)
	if err != nil {
		return nil, err
	}

	return &mediav1.GetAudioResponse{Data: data}, nil
}

func (h *MediaHandler) DeleteAudio(ctx context.Context, req *mediav1.DeleteAudioRequest) (*emptypb.Empty, error) {
	ownerID, err := parseID(req.GetOwnerId(), "owner id")
	if err != nil {
		return nil, err
	}

	if len(req.GetAudioIds()) > service.MaxPhotoIDs {
		return nil, status.Error(codes.InvalidArgument, "too many audio ids")
	}

	ids := make([]uuid.UUID, 0, len(req.GetAudioIds()))
	for _, raw := range req.GetAudioIds() {
		id, err := parseID(raw, "audio id")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err = h.audio.Delete(ctx, ownerID, ids); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
