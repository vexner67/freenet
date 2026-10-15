package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDocuments(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	id := uuid.Must(uuid.NewV7()).String()

	added, err := s.client.AddDocument(s.as(t, ann), &postv1.AddDocumentRequest{Id: id, FileName: " Отчёт за  год.PDF ", Size: 2048})
	if err != nil {
		t.Fatalf("AddDocument: %v", err)
	}
	doc := added.GetDocument()
	if doc.GetId() != id || doc.GetOwnerId() != ann.String() || doc.GetName() != "Отчёт за год.PDF" || doc.GetExt() != "pdf" || doc.GetSize() != 2048 {
		t.Fatalf("AddDocument = %v", doc)
	}

	for name, req := range map[string]*postv1.AddDocumentRequest{
		"a format openvk does not allow": {Id: uuid.NewString(), FileName: "virus.exe", Size: 1},
		"no bytes":                       {Id: uuid.NewString(), FileName: "a.txt"},
		"a bad id":                       {Id: "x", FileName: "a.txt", Size: 1},
	} {
		if _, err = s.client.AddDocument(s.as(t, ann), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("AddDocument with %s: %v", name, err)
		}
	}
	if _, err = s.client.AddDocument(context.Background(), &postv1.AddDocumentRequest{Id: uuid.NewString(), FileName: "a.txt", Size: 1}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("AddDocument without a token: %v", err)
	}

	// Only the caller's own are listed; anyone gets them by ID.
	list, err := s.client.ListDocuments(s.as(t, ann), &postv1.ListDocumentsRequest{})
	if err != nil || len(list.GetDocuments()) != 1 || list.GetTotal() != 1 {
		t.Fatalf("ListDocuments = %v, %v", list, err)
	}
	if list, err = s.client.ListDocuments(s.as(t, bob), &postv1.ListDocumentsRequest{}); err != nil || list.GetTotal() != 0 {
		t.Fatalf("ListDocuments of another = %v, %v", list, err)
	}
	got, err := s.client.GetDocuments(context.Background(), &postv1.GetDocumentsRequest{Ids: []string{id, uuid.NewString()}})
	if err != nil || len(got.GetDocuments()) != 1 {
		t.Fatalf("GetDocuments = %v, %v", got, err)
	}

	// A post of documents alone.
	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{DocumentIds: []string{id}})
	if err != nil || len(created.GetPost().GetDocumentIds()) != 1 || created.GetPost().GetDocumentIds()[0] != id {
		t.Fatalf("CreatePost with a document = %v, %v", created, err)
	}
	if _, err = s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{DocumentIds: []string{id, id}}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("CreatePost with a document twice: %v", err)
	}

	// The edit keeps them.
	edited, err := s.client.EditPost(s.as(t, ann), &postv1.EditPostRequest{Id: created.GetPost().GetId(), Text: "Вот"})
	if err != nil || len(edited.GetPost().GetDocumentIds()) != 1 {
		t.Fatalf("EditPost = %v, %v", edited, err)
	}

	if _, err = s.client.RenameDocument(s.as(t, bob), &postv1.RenameDocumentRequest{Id: id, Name: "Моё"}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("RenameDocument by another: %v", err)
	}
	renamed, err := s.client.RenameDocument(s.as(t, ann), &postv1.RenameDocumentRequest{Id: id, Name: "Отчёт"})
	if err != nil || renamed.GetDocument().GetName() != "Отчёт" {
		t.Fatalf("RenameDocument = %v, %v", renamed, err)
	}

	if _, err = s.client.DeleteDocument(s.as(t, bob), &postv1.DeleteDocumentRequest{Id: id}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("DeleteDocument by another: %v", err)
	}
	if _, err = s.client.DeleteDocument(s.as(t, ann), &postv1.DeleteDocumentRequest{Id: id}); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	if _, err = s.client.DeleteDocument(s.as(t, ann), &postv1.DeleteDocumentRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Errorf("DeleteDocument again: %v", err)
	}
}
