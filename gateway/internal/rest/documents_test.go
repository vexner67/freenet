package rest_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

var documentTime = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// fakeDocuments knows d1, Ann's (u1) report.pdf, and d2, Bob's (u2) notes.txt.
type fakeDocuments struct {
	token, added, fileName, renamed, deleted, cursor string
	size                                             int64
	// asked are the IDs of each GetMany, as many as there were.
	asked [][]string
	err   error
}

func (f *fakeDocuments) known() []model.Document {
	return []model.Document{
		{ID: "d1", OwnerID: "u1", Name: "report.pdf", Ext: "pdf", Size: 2048, CreatedAt: documentTime},
		{ID: "d2", OwnerID: "u2", Name: "notes", Ext: "txt", Size: 12, CreatedAt: documentTime},
	}
}

func (f *fakeDocuments) Add(_ context.Context, token, id, fileName string, size int64) (model.Document, error) {
	f.token, f.added, f.fileName, f.size = token, id, fileName, size
	return model.Document{ID: id, OwnerID: "u1", Name: fileName, Ext: "pdf", Size: size, CreatedAt: documentTime}, f.err
}

func (f *fakeDocuments) List(_ context.Context, token, cursor string, _ int) (model.DocumentPage, error) {
	f.token, f.cursor = token, cursor
	return model.DocumentPage{Documents: f.known()[:1], NextCursor: "d1", Total: 1}, f.err
}

func (f *fakeDocuments) GetMany(_ context.Context, ids []string) ([]model.Document, error) {
	f.asked = append(f.asked, ids)

	var found []model.Document
	for _, d := range f.known() {
		if slices.Contains(ids, d.ID) {
			found = append(found, d)
		}
	}

	return found, f.err
}

func (f *fakeDocuments) Rename(_ context.Context, token, id, name string) (model.Document, error) {
	f.token, f.renamed = token, id+"="+name
	d := f.known()[0]
	d.Name = name
	return d, f.err
}

func (f *fakeDocuments) Delete(_ context.Context, token, id string) error {
	f.token, f.deleted = token, id
	return f.err
}

type documentBody struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Ext       string `json:"ext"`
	Size      int64  `json:"size"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt"`
}

func TestUploadDocument(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/me/documents?name=report.pdf", "%PDF-1.7", "Authorization", "Bearer tok")
	got := decode[documentBody](t, w)

	if w.Code != http.StatusCreated || string(h.media.uploaded) != "%PDF-1.7" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if h.documents.added != "d9" || h.documents.fileName != "report.pdf" || h.documents.size != 8 || h.documents.token != "tok" {
		t.Fatalf("added = %+v", h.documents)
	}
	if got.ID != "d9" || got.URL != "/media/documents/d9" || got.CreatedAt != "2026-10-15T12:00:00Z" {
		t.Fatalf("document = %+v", got)
	}

	// The post service refuses the format: media lets the file go.
	h = newHarness(rest.Config{})
	h.documents.err = failure.New(failure.InvalidArgument, "document format is not allowed")
	w = h.do("POST", "/api/v1/me/documents?name=virus.exe", "MZ", "Authorization", "Bearer tok")
	if w.Code != http.StatusBadRequest || decode[errorBody](t, w).Error.Message != "document format is not allowed" {
		t.Fatalf("refused: %d %s", w.Code, w.Body)
	}
	if !slices.Equal(h.media.droppedDocuments, []string{"u1/d9"}) {
		t.Fatalf("dropped = %v", h.media.droppedDocuments)
	}

	// Empty, too big, anonymous.
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/me/documents?name=a.txt", "", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("empty: %d", w.Code)
	}
	big := string(make([]byte, 10<<20+1))
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/me/documents?name=a.txt", big, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("too big: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/me/documents?name=a.txt", "x"); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}
}

func TestDocuments(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/me/documents?cursor=d5", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Documents  []documentBody `json:"documents"`
		NextCursor string         `json:"nextCursor"`
		Total      int            `json:"total"`
	}](t, w)
	if w.Code != http.StatusOK || len(got.Documents) != 1 || got.Total != 1 || got.NextCursor != "d1" || h.documents.cursor != "d5" {
		t.Fatalf("list: %d %+v", w.Code, got)
	}

	w = h.do("PATCH", "/api/v1/documents/d1", `{"name":"отчёт"}`, "Authorization", "Bearer tok")
	if d := decode[documentBody](t, w); w.Code != http.StatusOK || d.Name != "отчёт" || h.documents.renamed != "d1=отчёт" {
		t.Fatalf("rename: %d %+v", w.Code, d)
	}

	if w = h.do("DELETE", "/api/v1/documents/d1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.documents.deleted != "d1" {
		t.Fatalf("delete: %d", w.Code)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/me/documents", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}
}

func TestServeDocument(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/media/documents/d1", "")
	if w.Code != http.StatusOK || w.Body.String() != "%PDF-1.7" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers = %v", w.Header())
	}
	if got := w.Header().Get("Content-Disposition"); got != `attachment; filename=report.pdf` {
		t.Fatalf("disposition = %q", got)
	}

	if w = h.do("GET", "/media/documents/nope", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", w.Code)
	}
}

func TestAttachDocuments(t *testing.T) {
	t.Parallel()

	filled := func() harness {
		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		return h
	}

	t.Run("to a post", func(t *testing.T) {
		t.Parallel()

		h := filled()
		w := h.do("POST", "/api/v1/me/posts", `{"text":"вот","documents":["d1"]}`, "Authorization", "Bearer tok")
		got := decode[struct {
			Documents []documentBody `json:"documents"`
		}](t, w)
		if w.Code != http.StatusCreated || !slices.Equal(h.posts.newPost.DocumentIDs, []string{"d1"}) {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		if len(got.Documents) != 1 || got.Documents[0].Name != "report.pdf" || got.Documents[0].URL != "/media/documents/d1" {
			t.Fatalf("documents = %+v", got.Documents)
		}
	})

	t.Run("only the caller's", func(t *testing.T) {
		t.Parallel()

		for _, body := range []string{
			`{"text":"x","documents":["d2"]}`,
			`{"text":"x","documents":["nope"]}`,
			`{"text":"x","documents":["d1","d1","d1","d1","d1","d1","d1","d1","d1","d1","d1"]}`,
		} {
			h := filled()
			if w := h.do("POST", "/api/v1/me/posts", body, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.posts.created != "" {
				t.Fatalf("%s: %d", body, w.Code)
			}
		}
	})

	t.Run("not by an edit", func(t *testing.T) {
		t.Parallel()

		h := filled()
		if w := h.do("PATCH", "/api/v1/posts/p1", `{"text":"x","documents":["d1"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	})

	t.Run("to a message", func(t *testing.T) {
		t.Parallel()

		h := filled()
		w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"sealed":"cHJpdmV0","documents":["d1"]}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)
		docs, _ := got["documents"].([]any)
		if w.Code != http.StatusCreated || len(docs) != 1 || got["documentCount"] != float64(1) {
			t.Fatalf("%d %v", w.Code, got)
		}

		h = filled()
		if w = h.do("POST", "/api/v1/dialogs/bob/messages", `{"sealed":"cHJpdmV0","documents":["d2"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
			t.Fatalf("someone else's: %d", w.Code)
		}
	})

	t.Run("to a chat message", func(t *testing.T) {
		t.Parallel()

		h := filled()
		w := h.do("POST", "/api/v1/chats/c1/messages", `{"text":"вот","documents":["d1"]}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)
		if docs, _ := got["documents"].([]any); w.Code != http.StatusCreated || len(docs) != 1 || !slices.Equal(h.chats.draft.DocumentIDs, []string{"d1"}) {
			t.Fatalf("%d %v", w.Code, got)
		}
	})
}
