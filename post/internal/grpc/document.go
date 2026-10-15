package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type DocumentService interface {
	Add(ctx context.Context, in service.NewDocumentInput) (*post.Document, error)
	List(ctx context.Context, ownerID, cursor uuid.UUID, size int) (service.DocumentPage, error)
	ByID(ctx context.Context, ids []uuid.UUID) ([]*post.Document, error)
	Rename(ctx context.Context, requesterID, id uuid.UUID, name post.DocumentName) (*post.Document, error)
	Delete(ctx context.Context, requesterID, id uuid.UUID) error
}

// AddDocument keeps a file the media service has stored for the caller,
// named after the file and of its format.
func (h *PostHandler) AddDocument(ctx context.Context, req *postv1.AddDocumentRequest) (*postv1.AddDocumentResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseDocumentID(req.GetId())
	if err != nil {
		return nil, err
	}

	ext, err := post.NewDocumentExt(req.GetFileName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	name, err := post.NewDocumentName(req.GetFileName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	d, err := h.documentService.Add(ctx, service.NewDocumentInput{ID: id, OwnerID: userID, Name: name, Ext: ext, Size: req.GetSize()})
	if err != nil {
		return nil, err
	}

	return &postv1.AddDocumentResponse{Document: toProtoDocument(d)}, nil
}

func (h *PostHandler) ListDocuments(ctx context.Context, req *postv1.ListDocumentsRequest) (*postv1.ListDocumentsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.documentService.List(ctx, userID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListDocumentsResponse{
		Documents: toProtoDocuments(page.Documents),
		Total:     int32(page.Total),
	}
	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

func (h *PostHandler) GetDocuments(ctx context.Context, req *postv1.GetDocumentsRequest) (*postv1.GetDocumentsResponse, error) {
	ids, err := parseIDs(req.GetIds())
	if err != nil {
		return nil, err
	}

	found, err := h.documentService.ByID(ctx, ids)
	if err != nil {
		return nil, err
	}

	return &postv1.GetDocumentsResponse{Documents: toProtoDocuments(found)}, nil
}

func (h *PostHandler) RenameDocument(ctx context.Context, req *postv1.RenameDocumentRequest) (*postv1.RenameDocumentResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseDocumentID(req.GetId())
	if err != nil {
		return nil, err
	}

	name, err := post.NewDocumentName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	d, err := h.documentService.Rename(ctx, userID, id, name)
	if err != nil {
		return nil, err
	}

	return &postv1.RenameDocumentResponse{Document: toProtoDocument(d)}, nil
}

func (h *PostHandler) DeleteDocument(ctx context.Context, req *postv1.DeleteDocumentRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseDocumentID(req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.documentService.Delete(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func parseDocumentID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid document id")
	}

	return id, nil
}

// fromProtoDocuments reads the documents a post attaches.
func fromProtoDocuments(raw []string) (post.Documents, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := parseDocumentID(s)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	documents, err := post.NewDocuments(ids)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return documents, nil
}

func toProtoDocuments(documents []*post.Document) []*postv1.Document {
	out := make([]*postv1.Document, len(documents))
	for i, d := range documents {
		out[i] = toProtoDocument(d)
	}

	return out
}

func toProtoDocument(d *post.Document) *postv1.Document {
	return &postv1.Document{
		Id:        d.ID().String(),
		OwnerId:   d.OwnerID().String(),
		Name:      d.Name().String(),
		Ext:       d.Ext().String(),
		Size:      d.Size(),
		CreatedAt: timestamppb.New(d.CreatedAt()),
	}
}

func idStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}

	return out
}
