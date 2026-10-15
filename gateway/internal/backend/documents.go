package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Documents is rest.Documents over the post service, which keeps what the
// documents of people are; their bytes are the media service's.
type Documents struct {
	api postv1.PostServiceClient
}

func NewDocuments(api postv1.PostServiceClient) *Documents {
	return &Documents{
		api: api,
	}
}

// Add keeps the file the media service stored for the caller as id.
func (d *Documents) Add(ctx context.Context, accessToken, id, fileName string, size int64) (model.Document, error) {
	resp, err := d.api.AddDocument(withToken(ctx, accessToken), &postv1.AddDocumentRequest{Id: id, FileName: fileName, Size: size})
	if err != nil {
		return model.Document{}, errs.Wrap("d.api.AddDocument", fromStatus(err))
	}

	return toDocument(resp.GetDocument()), nil
}

// List pages the caller's documents, newest first.
func (d *Documents) List(ctx context.Context, accessToken, cursor string, limit int) (model.DocumentPage, error) {
	resp, err := d.api.ListDocuments(withToken(ctx, accessToken), &postv1.ListDocumentsRequest{Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.DocumentPage{}, errs.Wrap("d.api.ListDocuments", fromStatus(err))
	}

	page := model.DocumentPage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, pd := range resp.GetDocuments() {
		page.Documents = append(page.Documents, toDocument(pd))
	}

	return page, nil
}

// GetMany returns the documents of ids that exist, at most 50, in no
// particular order.
func (d *Documents) GetMany(ctx context.Context, ids []string) ([]model.Document, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	resp, err := d.api.GetDocuments(ctx, &postv1.GetDocumentsRequest{Ids: ids})
	if err != nil {
		return nil, errs.Wrap("d.api.GetDocuments", fromStatus(err))
	}

	found := make([]model.Document, 0, len(resp.GetDocuments()))
	for _, pd := range resp.GetDocuments() {
		found = append(found, toDocument(pd))
	}

	return found, nil
}

func (d *Documents) Rename(ctx context.Context, accessToken, id, name string) (model.Document, error) {
	resp, err := d.api.RenameDocument(withToken(ctx, accessToken), &postv1.RenameDocumentRequest{Id: id, Name: name})
	if err != nil {
		return model.Document{}, errs.Wrap("d.api.RenameDocument", fromStatus(err))
	}

	return toDocument(resp.GetDocument()), nil
}

func (d *Documents) Delete(ctx context.Context, accessToken, id string) error {
	if _, err := d.api.DeleteDocument(withToken(ctx, accessToken), &postv1.DeleteDocumentRequest{Id: id}); err != nil {
		return errs.Wrap("d.api.DeleteDocument", fromStatus(err))
	}

	return nil
}

func toDocument(pd *postv1.Document) model.Document {
	return model.Document{
		ID:        pd.GetId(),
		OwnerID:   pd.GetOwnerId(),
		Name:      pd.GetName(),
		Ext:       pd.GetExt(),
		Size:      pd.GetSize(),
		CreatedAt: pd.GetCreatedAt().AsTime(),
	}
}
