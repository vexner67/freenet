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

type DocumentService interface {
	Upload(ctx context.Context, ownerID uuid.UUID, data []byte) (uuid.UUID, error)
	Get(ctx context.Context, ownerID, documentID uuid.UUID) ([]byte, error)
	Delete(ctx context.Context, ownerID uuid.UUID, documentIDs []uuid.UUID) error
}

func (h *MediaHandler) UploadDocument(ctx context.Context, req *mediav1.UploadDocumentRequest) (*mediav1.UploadDocumentResponse, error) {
	ownerID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := h.documents.Upload(ctx, ownerID, req.GetData())
	if err != nil {
		return nil, err
	}

	return &mediav1.UploadDocumentResponse{DocumentId: id.String()}, nil
}

func (h *MediaHandler) GetDocument(ctx context.Context, req *mediav1.GetDocumentRequest) (*mediav1.GetDocumentResponse, error) {
	ownerID, err := parseID(req.GetOwnerId(), "owner id")
	if err != nil {
		return nil, err
	}

	documentID, err := parseID(req.GetDocumentId(), "document id")
	if err != nil {
		return nil, err
	}

	data, err := h.documents.Get(ctx, ownerID, documentID)
	if err != nil {
		return nil, err
	}

	return &mediav1.GetDocumentResponse{Data: data}, nil
}

func (h *MediaHandler) DeleteDocuments(ctx context.Context, req *mediav1.DeleteDocumentsRequest) (*emptypb.Empty, error) {
	ownerID, err := parseID(req.GetOwnerId(), "owner id")
	if err != nil {
		return nil, err
	}

	if len(req.GetDocumentIds()) > service.MaxPhotoIDs {
		return nil, status.Error(codes.InvalidArgument, "too many document ids")
	}

	ids := make([]uuid.UUID, 0, len(req.GetDocumentIds()))
	for _, raw := range req.GetDocumentIds() {
		id, err := parseID(raw, "document id")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err = h.documents.Delete(ctx, ownerID, ids); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
