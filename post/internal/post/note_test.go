package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewNoteTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{name: "plain", in: "Про котов", want: "Про котов"},
		{name: "one line, trimmed", in: "  Про \n котов\t ", want: "Про котов"},
		{name: "at the limit", in: strings.Repeat("я", 256), want: strings.Repeat("я", 256)},
		{name: "empty", in: " \n ", err: post.ErrNoteTitleEmpty},
		{name: "too long", in: strings.Repeat("я", 257), err: post.ErrNoteTitleTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewNoteTitle(tt.in)
			if !errors.Is(err, tt.err) || got.String() != tt.want {
				t.Fatalf("NewNoteTitle = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestNewNoteText(t *testing.T) {
	t.Parallel()

	if got, err := post.NewNoteText("  первая строка\r\nвторая 🐈  "); err != nil || got.String() != "первая строка\nвторая 🐈" {
		t.Fatalf("NewNoteText = %q, %v", got, err)
	}

	if _, err := post.NewNoteText("   "); !errors.Is(err, post.ErrNoteEmpty) {
		t.Fatalf("an empty note: %v", err)
	}

	if _, err := post.NewNoteText(strings.Repeat("я", 60001)); !errors.Is(err, post.ErrNoteTooLong) {
		t.Fatalf("a note too long: %v", err)
	}

	if _, err := post.NewNoteText(strings.Repeat("я", 60000)); err != nil {
		t.Fatalf("a note at the limit: %v", err)
	}
}

func TestNote(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	now := time.Date(2026, 10, 11, 12, 0, 0, 123456789, time.FixedZone("msk", 3*3600))

	n, err := post.NewNote(ann, "Про котов", "Коты спят.", now)
	if err != nil {
		t.Fatal(err)
	}

	if n.ID() == uuid.Nil || n.AuthorID() != ann || n.Title() != "Про котов" || n.Text() != "Коты спят." || n.Comments() != 0 {
		t.Fatalf("NewNote = %+v", n)
	}
	if !n.CreatedAt().Equal(now.Truncate(time.Microsecond)) || n.CreatedAt().Location() != time.UTC || !n.EditedAt().IsZero() {
		t.Fatalf("created at %v, edited at %v", n.CreatedAt(), n.EditedAt())
	}

	if _, err = n.Edited(bob, "Чужое", "Чужое.", now); !errors.Is(err, post.ErrCannotChangeNote) {
		t.Fatalf("a stranger edits: %v", err)
	}

	later := now.Add(400 * 24 * time.Hour)
	edited, err := n.Edited(ann, "Про собак", "Собаки лают.", later)
	if err != nil {
		t.Fatalf("the author edits a year later: %v", err)
	}
	if edited.Title() != "Про собак" || edited.Text() != "Собаки лают." || !edited.EditedAt().Equal(later.Truncate(time.Microsecond)) || edited.ID() != n.ID() {
		t.Fatalf("Edited = %+v", edited)
	}
	if n.Title() != "Про котов" || !n.EditedAt().IsZero() {
		t.Fatal("editing changed the note handed out")
	}

	if counted := n.WithComments(3); counted.Comments() != 3 || n.Comments() != 0 {
		t.Fatalf("WithComments = %d, the note has %d", counted.Comments(), n.Comments())
	}

	restored := post.RestoreNote(post.StoredNote{
		ID: n.ID(), AuthorID: ann, Title: "Про котов", Text: "Коты спят.", Comments: 2, CreatedAt: n.CreatedAt(), EditedAt: later,
	})
	if restored.Comments() != 2 || !restored.EditedAt().Equal(later) || !restored.CanBeChangedBy(ann) || restored.CanBeChangedBy(bob) {
		t.Fatalf("RestoreNote = %+v", restored)
	}
}

func TestNoteComment(t *testing.T) {
	t.Parallel()

	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()

	n, err := post.NewNote(ann, "Про котов", "Коты спят.", now)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = post.NewNoteComment(n.ID(), bob, "", now); !errors.Is(err, post.ErrCommentEmpty) {
		t.Fatalf("an empty comment: %v", err)
	}

	c, err := post.NewNoteComment(n.ID(), bob, "И мурчат", now)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID() == uuid.Nil || c.NoteID() != n.ID() || c.AuthorID() != bob || c.Text() != "И мурчат" {
		t.Fatalf("NewNoteComment = %+v", c)
	}

	tests := []struct {
		name string
		who  uuid.UUID
		want bool
	}{
		{"its author", bob, true},
		{"the author of the note", ann, true},
		{"a stranger", cat, false},
	}
	for _, tt := range tests {
		if got := c.CanBeDeletedBy(tt.who, n); got != tt.want {
			t.Errorf("CanBeDeletedBy(%s) = %v", tt.name, got)
		}
	}
}
