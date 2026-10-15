package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewDocumentName(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, in string
		want     post.DocumentName
		err      error
	}{
		{name: "trimmed to one line", in: "  Отчёт\n за   год.pdf ", want: "Отчёт за год.pdf"},
		{name: "empty", in: " \n ", err: post.ErrDocumentNameEmpty},
		{name: "at the limit", in: strings.Repeat("я", 128), want: post.DocumentName(strings.Repeat("я", 128))},
		{name: "too long", in: strings.Repeat("я", 129), err: post.ErrDocumentNameTooLong},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewDocumentName(tt.in)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("NewDocumentName(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
			}
		})
	}
}

func TestNewDocumentExt(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   string
		want post.DocumentExt
		err  error
	}{
		{in: "pdf", want: "pdf"},
		{in: "Отчёт.PDF", want: "pdf"},
		{in: " archive.tar.7z ", want: "7z"},
		{in: ".docx", want: "docx"},
		{in: "script.exe", err: post.ErrDocumentFormat},
		{in: "page.html", err: post.ErrDocumentFormat},
		{in: "noext", err: post.ErrDocumentFormat},
		{in: "", err: post.ErrDocumentFormat},
	} {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewDocumentExt(tt.in)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("NewDocumentExt(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
			}
		})
	}
}

func TestDocument(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	now := time.Date(2026, 10, 15, 12, 0, 0, 123456789, time.UTC)

	if _, err := post.NewDocument(uuid.Nil, ann, "a.pdf", "pdf", 10, now); !errors.Is(err, post.ErrInvalidDocument) {
		t.Fatalf("without an ID: %v", err)
	}
	if _, err := post.NewDocument(uuid.New(), ann, "a.pdf", "pdf", 0, now); !errors.Is(err, post.ErrInvalidDocumentSize) {
		t.Fatalf("empty: %v", err)
	}

	d, err := post.NewDocument(uuid.New(), ann, "a.pdf", "pdf", 10, now)
	if err != nil || !d.CreatedAt().Equal(now.Truncate(time.Microsecond)) || d.Size() != 10 {
		t.Fatalf("NewDocument = %+v, %v", d, err)
	}

	if _, err = d.Renamed(bob, "b.pdf"); !errors.Is(err, post.ErrCannotChangeDocument) {
		t.Fatalf("renamed by another: %v", err)
	}

	renamed, err := d.Renamed(ann, "Договор")
	if err != nil || renamed.Name() != "Договор" || d.Name() != "a.pdf" || renamed.Ext() != "pdf" {
		t.Fatalf("Renamed = %+v, %v; the old one %q", renamed, err, d.Name())
	}
}

func TestNewDocuments(t *testing.T) {
	t.Parallel()

	one := uuid.New()
	ids := make([]uuid.UUID, 11)
	for i := range ids {
		ids[i] = uuid.New()
	}

	if _, err := post.NewDocuments(ids); !errors.Is(err, post.ErrTooManyDocuments) {
		t.Fatalf("eleven: %v", err)
	}
	if _, err := post.NewDocuments([]uuid.UUID{one, one}); !errors.Is(err, post.ErrDuplicateDocument) {
		t.Fatalf("twice: %v", err)
	}
	if _, err := post.NewDocuments([]uuid.UUID{uuid.Nil}); !errors.Is(err, post.ErrInvalidDocument) {
		t.Fatalf("nil: %v", err)
	}
	if got, err := post.NewDocuments(ids[:10]); err != nil || len(got) != 10 {
		t.Fatalf("ten = %v, %v", got, err)
	}
}

func TestContent_Documents(t *testing.T) {
	t.Parallel()

	docs := post.Documents{uuid.New()}

	if _, err := post.NewDocumented("", nil, nil); !errors.Is(err, post.ErrTextEmpty) {
		t.Fatalf("nothing at all: %v", err)
	}

	c, err := post.NewDocumented("", nil, docs)
	if err != nil || len(c.Documents()) != 1 {
		t.Fatalf("documents alone = %+v, %v", c, err)
	}

	// An edit keeps the documents, so the words may go.
	p, err := post.New(uuid.New(), uuid.New(), c, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	edited, err := p.Edited(p.AuthorID(), "", nil, time.Now())
	if err != nil || len(edited.Content().Documents()) != 1 {
		t.Fatalf("edited = %+v, %v", edited, err)
	}
}
