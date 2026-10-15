package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// fakeDocumentKeeper records the documents media was told to delete, and
// in how many calls it was not in a transaction.
type fakeDocumentKeeper struct {
	deleted   []photoCall
	outsideTx int
	err       error
}

func (f *fakeDocumentKeeper) DeleteDocuments(ctx context.Context, owner uuid.UUID, ids []uuid.UUID) error {
	if ctx.Value(inTx{}) == nil {
		f.outsideTx++
	}
	if f.err != nil {
		return f.err
	}

	f.deleted = append(f.deleted, photoCall{owner, ids})

	return nil
}

func addDocument(t *testing.T, svc *service.DocumentService, owner uuid.UUID, file string) *post.Document {
	t.Helper()

	name, _ := post.NewDocumentName(file)
	ext, err := post.NewDocumentExt(file)
	if err != nil {
		t.Fatal(err)
	}

	d, err := svc.Add(context.Background(), service.NewDocumentInput{ID: uuid.Must(uuid.NewV7()), OwnerID: owner, Name: name, Ext: ext, Size: 1024})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	return d
}

func TestDocumentService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keeper := &fakeDocumentKeeper{}
	svc := service.NewDocumentService(inmem.NewDocumentRepository(), fakeTx{}, keeper)
	ann, bob := uuid.New(), uuid.New()

	first := addDocument(t, svc, ann, "Отчёт.PDF")
	second := addDocument(t, svc, ann, "notes.txt")
	addDocument(t, svc, bob, "bob.zip")

	if first.Name() != "Отчёт.PDF" || first.Ext() != "pdf" || first.OwnerID() != ann || first.Size() != 1024 {
		t.Fatalf("Add = %+v", first)
	}

	if _, err := svc.Add(ctx, service.NewDocumentInput{ID: uuid.New(), OwnerID: ann, Name: "a", Ext: "pdf"}); !errors.Is(err, post.ErrInvalidDocumentSize) {
		t.Fatalf("Add without bytes: %v", err)
	}

	// Newest first, paged.
	page, err := svc.List(ctx, ann, uuid.Nil, 1)
	if err != nil || len(page.Documents) != 1 || page.Documents[0].ID() != second.ID() || page.Total != 2 || page.NextCursor != second.ID() {
		t.Fatalf("the first page = %+v, %v", page, err)
	}
	if page, err = svc.List(ctx, ann, page.NextCursor, 1); err != nil || len(page.Documents) != 1 || page.Documents[0].ID() != first.ID() || page.NextCursor != uuid.Nil {
		t.Fatalf("the second page = %+v, %v", page, err)
	}

	found, err := svc.ByID(ctx, []uuid.UUID{first.ID(), uuid.New()})
	if err != nil || len(found) != 1 || found[0].ID() != first.ID() {
		t.Fatalf("ByID = %v, %v", found, err)
	}
	if _, err = svc.ByID(ctx, make([]uuid.UUID, service.MaxPageSize+1)); !errors.Is(err, service.ErrTooManyIDs) {
		t.Fatalf("ByID with too many: %v", err)
	}

	// Only the owner renames and deletes.
	if _, err = svc.Rename(ctx, bob, first.ID(), "Чужое"); !errors.Is(err, post.ErrCannotChangeDocument) {
		t.Fatalf("Rename by another: %v", err)
	}
	renamed, err := svc.Rename(ctx, ann, first.ID(), "Годовой отчёт")
	if err != nil || renamed.Name() != "Годовой отчёт" || renamed.Ext() != "pdf" {
		t.Fatalf("Rename = %v, %v", renamed, err)
	}

	if err = svc.Delete(ctx, bob, first.ID()); !errors.Is(err, post.ErrCannotChangeDocument) {
		t.Fatalf("Delete by another: %v", err)
	}
	if err = svc.Delete(ctx, ann, first.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(keeper.deleted) != 1 || keeper.deleted[0].owner != ann || keeper.deleted[0].photos[0] != first.ID() || keeper.outsideTx != 0 {
		t.Fatalf("media was told %+v, outside a transaction %d times", keeper.deleted, keeper.outsideTx)
	}
	if err = svc.Delete(ctx, ann, first.ID()); !errors.Is(err, post.ErrDocumentNotFound) {
		t.Fatalf("Delete again: %v", err)
	}
	if _, err = svc.Rename(ctx, ann, first.ID(), "x"); !errors.Is(err, post.ErrDocumentNotFound) {
		t.Fatalf("Rename a deleted one: %v", err)
	}

	// Delete fails when media cannot be told.
	keeper.err = errors.New("outbox is down")
	if err = svc.Delete(ctx, ann, second.ID()); err == nil {
		t.Fatal("Delete succeeded without the outbox")
	}
}
