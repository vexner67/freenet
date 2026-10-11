package grpc_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNotes(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	anon := context.Background()

	created, err := s.client.CreateNote(s.as(t, ann), &postv1.CreateNoteRequest{Title: "  Про \n котов ", Text: "Коты спят и мурчат."})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	note := created.GetNote()
	if note.GetTitle() != "Про котов" || note.GetAuthorId() != ann.String() || !note.GetMine() || note.GetEditedAt() != nil {
		t.Fatalf("CreateNote = %v", note)
	}

	if _, err = s.client.CreateNote(s.as(t, ann), &postv1.CreateNoteRequest{Title: "Вторая", Text: "Да"}); err != nil {
		t.Fatal(err)
	}

	// Anyone reads a note and the list, whole or cut.
	got, err := s.client.GetNote(anon, &postv1.GetNoteRequest{Id: note.GetId()})
	if err != nil || got.GetNote().GetText() != "Коты спят и мурчат." || got.GetNote().GetMine() || got.GetNote().GetTextCut() {
		t.Fatalf("GetNote = %v, %v", got, err)
	}

	list, err := s.client.ListNotes(s.as(t, bob), &postv1.ListNotesRequest{AuthorId: ann.String(), TextLimit: 4})
	if err != nil || len(list.GetNotes()) != 2 || list.GetTotal() != 2 {
		t.Fatalf("ListNotes = %v, %v", list, err)
	}
	if short, long := list.GetNotes()[0], list.GetNotes()[1]; short.GetText() != "Да" || short.GetTextCut() ||
		long.GetText() != "Коты" || !long.GetTextCut() || long.GetMine() {
		t.Fatalf("ListNotes texts = %v", list.GetNotes())
	}

	page, err := s.client.ListNotes(anon, &postv1.ListNotesRequest{AuthorId: ann.String(), Limit: 1})
	if err != nil || len(page.GetNotes()) != 1 || page.GetNextCursor() == "" {
		t.Fatalf("the first page = %v, %v", page, err)
	}
	if page, err = s.client.ListNotes(anon, &postv1.ListNotesRequest{AuthorId: ann.String(), Limit: 1, Cursor: page.GetNextCursor()}); err != nil ||
		len(page.GetNotes()) != 1 || page.GetNotes()[0].GetId() != note.GetId() || page.GetNextCursor() != "" {
		t.Fatalf("the second page = %v, %v", page, err)
	}

	edited, err := s.client.EditNote(s.as(t, ann), &postv1.EditNoteRequest{Id: note.GetId(), Title: "Про собак", Text: "Собаки лают."})
	if err != nil || edited.GetNote().GetTitle() != "Про собак" || edited.GetNote().GetEditedAt() == nil {
		t.Fatalf("EditNote = %v, %v", edited, err)
	}

	comment, err := s.client.CreateNoteComment(s.as(t, bob), &postv1.CreateNoteCommentRequest{NoteId: note.GetId(), Text: "длинный ответ"})
	if err != nil || !comment.GetComment().GetMine() || comment.GetComment().GetNoteId() != note.GetId() {
		t.Fatalf("CreateNoteComment = %v, %v", comment, err)
	}
	commentID := comment.GetComment().GetId()

	comments, err := s.client.ListNoteComments(s.as(t, ann), &postv1.ListNoteCommentsRequest{NoteId: note.GetId(), TextLimit: 7})
	if err != nil || comments.GetTotal() != 1 || len(comments.GetComments()) != 1 {
		t.Fatalf("ListNoteComments = %v, %v", comments, err)
	}
	if c := comments.GetComments()[0]; c.GetText() != "длинный" || !c.GetTextCut() || c.GetMine() || c.GetAuthorId() != bob.String() {
		t.Fatalf("the listed comment = %v", c)
	}

	whole, err := s.client.GetNoteComment(anon, &postv1.GetNoteCommentRequest{Id: commentID})
	if err != nil || whole.GetComment().GetText() != "длинный ответ" || whole.GetComment().GetTextCut() {
		t.Fatalf("GetNoteComment = %v, %v", whole, err)
	}

	many, err := s.client.GetNoteComments(anon, &postv1.GetNoteCommentsRequest{Ids: []string{commentID, uuid.NewString()}, TextLimit: 7})
	if err != nil || len(many.GetComments()) != 1 || many.GetComments()[0].GetText() != "длинный" {
		t.Fatalf("GetNoteComments = %v, %v", many, err)
	}

	if after, _ := s.client.GetNote(anon, &postv1.GetNoteRequest{Id: note.GetId()}); after.GetNote().GetCommentCount() != 1 {
		t.Fatalf("the note counts %d comments", after.GetNote().GetCommentCount())
	}

	// The author of the note deletes a comment under it, then the note.
	if _, err = s.client.DeleteNoteComment(s.as(t, ann), &postv1.DeleteNoteCommentRequest{Id: commentID}); err != nil {
		t.Fatalf("DeleteNoteComment: %v", err)
	}
	if _, err = s.client.DeleteNote(s.as(t, ann), &postv1.DeleteNoteRequest{Id: note.GetId()}); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	if _, err = s.client.GetNote(anon, &postv1.GetNoteRequest{Id: note.GetId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("the deleted note: %v", err)
	}
}

func TestNotes_Errors(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	anon := context.Background()

	created, err := s.client.CreateNote(s.as(t, ann), &postv1.CreateNoteRequest{Title: "Про котов", Text: "Коты спят."})
	if err != nil {
		t.Fatal(err)
	}
	id := created.GetNote().GetId()

	comment, err := s.client.CreateNoteComment(s.as(t, bob), &postv1.CreateNoteCommentRequest{NoteId: id, Text: "ответ"})
	if err != nil {
		t.Fatal(err)
	}
	commentID := comment.GetComment().GetId()

	tests := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"create without a token", func() error {
			_, err := s.client.CreateNote(anon, &postv1.CreateNoteRequest{Title: "Т", Text: "т"})
			return err
		}, codes.Unauthenticated},
		{"create without a title", func() error {
			_, err := s.client.CreateNote(s.as(t, ann), &postv1.CreateNoteRequest{Title: " ", Text: "т"})
			return err
		}, codes.InvalidArgument},
		{"create with a title too long", func() error {
			_, err := s.client.CreateNote(s.as(t, ann), &postv1.CreateNoteRequest{Title: strings.Repeat("я", 257), Text: "т"})
			return err
		}, codes.InvalidArgument},
		{"create without a text", func() error {
			_, err := s.client.CreateNote(s.as(t, ann), &postv1.CreateNoteRequest{Title: "Т", Text: " \n "})
			return err
		}, codes.InvalidArgument},
		{"get with a bad id", func() error {
			_, err := s.client.GetNote(anon, &postv1.GetNoteRequest{Id: "x"})
			return err
		}, codes.InvalidArgument},
		{"get of no note", func() error {
			_, err := s.client.GetNote(anon, &postv1.GetNoteRequest{Id: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"list with a bad author", func() error {
			_, err := s.client.ListNotes(anon, &postv1.ListNotesRequest{AuthorId: "x"})
			return err
		}, codes.InvalidArgument},
		{"edit by a stranger", func() error {
			_, err := s.client.EditNote(s.as(t, bob), &postv1.EditNoteRequest{Id: id, Title: "Т", Text: "т"})
			return err
		}, codes.PermissionDenied},
		{"edit to nothing", func() error {
			_, err := s.client.EditNote(s.as(t, ann), &postv1.EditNoteRequest{Id: id, Title: "Т", Text: ""})
			return err
		}, codes.InvalidArgument},
		{"edit of no note", func() error {
			_, err := s.client.EditNote(s.as(t, ann), &postv1.EditNoteRequest{Id: uuid.NewString(), Title: "Т", Text: "т"})
			return err
		}, codes.NotFound},
		{"delete by a stranger", func() error {
			_, err := s.client.DeleteNote(s.as(t, bob), &postv1.DeleteNoteRequest{Id: id})
			return err
		}, codes.PermissionDenied},
		{"comment without a token", func() error {
			_, err := s.client.CreateNoteComment(anon, &postv1.CreateNoteCommentRequest{NoteId: id, Text: "т"})
			return err
		}, codes.Unauthenticated},
		{"an empty comment", func() error {
			_, err := s.client.CreateNoteComment(s.as(t, bob), &postv1.CreateNoteCommentRequest{NoteId: id, Text: "  "})
			return err
		}, codes.InvalidArgument},
		{"a comment under no note", func() error {
			_, err := s.client.CreateNoteComment(s.as(t, bob), &postv1.CreateNoteCommentRequest{NoteId: uuid.NewString(), Text: "т"})
			return err
		}, codes.NotFound},
		{"comments of no note", func() error {
			_, err := s.client.ListNoteComments(anon, &postv1.ListNoteCommentsRequest{NoteId: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"no such comment", func() error {
			_, err := s.client.GetNoteComment(anon, &postv1.GetNoteCommentRequest{Id: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"a comment deleted by a stranger", func() error {
			_, err := s.client.DeleteNoteComment(s.as(t, cat), &postv1.DeleteNoteCommentRequest{Id: commentID})
			return err
		}, codes.PermissionDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Code(tt.call()); got != tt.want {
				t.Fatalf("code = %v, want %v", got, tt.want)
			}
		})
	}
}

// Erasing a page takes its notes.
func TestNotes_Purge(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := uuid.New()

	created, err := s.client.CreateNote(s.as(t, ann), &postv1.CreateNoteRequest{Title: "Про котов", Text: "Коты спят."})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = s.client.PurgeUser(context.Background(), &postv1.PurgeUserRequest{UserId: ann.String()}); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}

	if _, err = s.client.GetNote(context.Background(), &postv1.GetNoteRequest{Id: created.GetNote().GetId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("the note of the erased: %v", err)
	}
}
