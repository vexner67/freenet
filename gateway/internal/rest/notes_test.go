package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakeNotes knows the notes note1 of u1 (ann), note3 of u2 (bob), note2
// of u3 (a deleted page) and the comments nc1 (of u2) and nc2 (of u3) under note1.
type fakeNotes struct {
	token, authorID, cursor string
	limit                   int
	created, edited         [2]string
	editedID, deleted       string
	// commented is "note/text" of the last comment, deletedComment the
	// comment deleted last.
	commented, deletedComment string
	err                       error
}

var noteTime = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func (f *fakeNotes) note(id string) (model.Note, error) {
	switch id {
	case "note1":
		return model.Note{ID: "note1", AuthorID: "u1", Title: "Про котов", Text: "Коты спят.", Comments: 2, CreatedAt: noteTime, EditedAt: noteTime.Add(time.Hour)}, nil
	case "note3":
		return model.Note{ID: "note3", AuthorID: "u2", Title: "Петра", Text: "Текст.", CreatedAt: noteTime}, nil
	case "note2":
		return model.Note{ID: "note2", AuthorID: "u3", Title: "Удалённой", Text: "Текст.", CreatedAt: noteTime}, nil
	default:
		return model.Note{}, failure.New(failure.NotFound, "note not found")
	}
}

func (f *fakeNotes) Create(_ context.Context, token, title, text string) (model.Note, error) {
	f.token, f.created = token, [2]string{title, text}
	return model.Note{ID: "note9", AuthorID: "u1", Title: title, Text: text, CreatedAt: noteTime, Mine: true}, f.err
}

func (f *fakeNotes) Get(_ context.Context, token, id string) (model.Note, error) {
	f.token = token
	if f.err != nil {
		return model.Note{}, f.err
	}
	return f.note(id)
}

func (f *fakeNotes) List(_ context.Context, token, authorID, cursor string, limit int) (model.NotePage, error) {
	f.token, f.authorID, f.cursor, f.limit = token, authorID, cursor, limit
	n, _ := f.note("note1")
	n.Text, n.TextCut = "Коты", true
	return model.NotePage{Notes: []model.Note{n}, NextCursor: "note0", Total: 4}, f.err
}

func (f *fakeNotes) Edit(_ context.Context, token, id, title, text string) (model.Note, error) {
	f.token, f.editedID, f.edited = token, id, [2]string{title, text}
	return model.Note{ID: id, AuthorID: "u1", Title: title, Text: text, CreatedAt: noteTime, EditedAt: noteTime, Mine: true}, f.err
}

func (f *fakeNotes) Delete(_ context.Context, token, id string) error {
	f.token, f.deleted = token, id
	return f.err
}

func (f *fakeNotes) Comment(_ context.Context, token, noteID, text string) (model.NoteComment, error) {
	f.token, f.commented = token, noteID+"/"+text
	return model.NoteComment{ID: "nc9", NoteID: noteID, AuthorID: "u1", Text: text, CreatedAt: noteTime, Mine: true}, f.err
}

func (f *fakeNotes) DeleteComment(_ context.Context, token, id string) error {
	f.token, f.deletedComment = token, id
	return f.err
}

func (f *fakeNotes) Comments(_ context.Context, token, noteID, cursor string, limit int) (model.NoteCommentPage, error) {
	f.token, f.cursor, f.limit = token, cursor, limit
	return model.NoteCommentPage{
		Comments: []model.NoteComment{
			{ID: "nc1", NoteID: noteID, AuthorID: "u2", Text: "мяу", TextCut: true, CreatedAt: noteTime},
			{ID: "nc2", NoteID: noteID, AuthorID: "u3", Text: "от удалённой", CreatedAt: noteTime},
		},
		NextCursor: "nc2", Total: 2,
	}, f.err
}

func (f *fakeNotes) CommentByID(_ context.Context, id string) (model.NoteComment, error) {
	switch id {
	case "nc1":
		return model.NoteComment{ID: "nc1", NoteID: "note1", AuthorID: "u2", Text: "мяу, целиком"}, f.err
	case "nc3":
		return model.NoteComment{ID: "nc3", NoteID: "note2", AuthorID: "u2", Text: "под удалённой"}, f.err
	default:
		return model.NoteComment{}, failure.New(failure.NotFound, "comment not found")
	}
}

// CommentsByID knows nc1.
func (f *fakeNotes) CommentsByID(_ context.Context, ids []string) ([]model.NoteComment, error) {
	var found []model.NoteComment
	for _, id := range ids {
		if id == "nc1" {
			found = append(found, model.NoteComment{ID: id, NoteID: "note1", AuthorID: "u2", Text: "мяу"})
		}
	}

	return found, f.err
}

type noteBody struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Text      string  `json:"text"`
	TextCut   bool    `json:"textCut"`
	Comments  int     `json:"comments"`
	CreatedAt string  `json:"createdAt"`
	EditedAt  *string `json:"editedAt"`
	Mine      bool    `json:"mine"`
}

func TestListUserNotes(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/users/ann/notes?cursor=note5&limit=3", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Author     map[string]any `json:"author"`
		Notes      []noteBody     `json:"notes"`
		NextCursor string         `json:"nextCursor"`
		Total      int            `json:"total"`
	}](t, w)

	if w.Code != http.StatusOK || got.Author["nickname"] != "ann" || got.NextCursor != "note0" || got.Total != 4 || len(got.Notes) != 1 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if h.notes.token != "tok" || h.notes.authorID != "u1" || h.notes.cursor != "note5" || h.notes.limit != 3 {
		t.Fatalf("request = %+v", h.notes)
	}
	if n := got.Notes[0]; n.ID != "note1" || n.Title != "Про котов" || n.Text != "Коты" || !n.TextCut || n.Comments != 2 ||
		n.CreatedAt != "2026-10-11T12:00:00Z" || n.EditedAt == nil || *n.EditedAt != "2026-10-11T13:00:00Z" {
		t.Fatalf("note = %+v", n)
	}

	// No token is needed; the notes of a deleted page and of nobody are
	// not shown.
	if w = newHarness(rest.Config{}).do("GET", "/api/v1/users/ann/notes", ""); w.Code != http.StatusOK {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("GET", "/api/v1/users/cat/notes", ""); w.Code != http.StatusForbidden {
		t.Fatalf("a deleted page: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("GET", "/api/v1/users/nobody/notes", ""); w.Code != http.StatusNotFound {
		t.Fatalf("nobody: %d", w.Code)
	}
}

func TestCreateNote(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

	w := h.do("POST", "/api/v1/me/notes", `{"title":"Про котов","text":"Коты спят."}`, "Authorization", "Bearer tok")
	got := decode[noteBody](t, w)
	if w.Code != http.StatusCreated || got.ID != "note9" || !got.Mine || got.EditedAt != nil || h.notes.created != [2]string{"Про котов", "Коты спят."} || h.notes.token != "tok" {
		t.Fatalf("%d %+v, created %v", w.Code, got, h.notes.created)
	}

	// Without a nickname there is no address for the notes.
	h = newHarness(rest.Config{})
	w = h.do("POST", "/api/v1/me/notes", `{"title":"Про котов","text":"Коты спят."}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusBadRequest || decode[errorBody](t, w).Error.Message != "fill in your profile before writing notes" || h.notes.created != [2]string{} {
		t.Fatalf("without a profile: %d %s", w.Code, w.Body)
	}

	h = newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	h.notes.err = failure.New(failure.InvalidArgument, "note title is empty")
	if w = h.do("POST", "/api/v1/me/notes", `{"title":"","text":"т"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("an empty title: %d", w.Code)
	}

	if w = newHarness(rest.Config{}).do("POST", "/api/v1/me/notes", `{"title":"Т","text":"т"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestGetNote(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/notes/note1", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Author map[string]any `json:"author"`
		Note   noteBody       `json:"note"`
	}](t, w)
	if w.Code != http.StatusOK || got.Author["nickname"] != "ann" || got.Note.Text != "Коты спят." || got.Note.TextCut ||
		h.notes.token != "tok" || h.users.accessAsked != "u1" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	tests := []struct {
		name string
		hide func(h *harness)
		path string
	}{
		{"no such note", func(*harness) {}, "/api/v1/notes/nope"},
		{"a note of a deleted page", func(*harness) {}, "/api/v1/notes/note2"},
		{"hidden notes", func(h *harness) { h.users.hiddenNotes = true }, "/api/v1/notes/note1"},
		{"a hidden page", func(h *harness) { h.users.hiddenPage = true }, "/api/v1/notes/note1"},
		{"the comments of hidden notes", func(h *harness) { h.users.hiddenNotes = true }, "/api/v1/notes/note1/comments"},
		{"a comment of hidden notes", func(h *harness) { h.users.hiddenNotes = true }, "/api/v1/note-comments/nc1/text"},
		{"a comment under a note of a deleted page", func(*harness) {}, "/api/v1/note-comments/nc3/text"},
		{"no such comment", func(*harness) {}, "/api/v1/note-comments/nope/text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{})
			tt.hide(&h)
			if w := h.do("GET", tt.path, ""); w.Code != http.StatusNotFound {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
}

func TestEditAndDeleteNote(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("PATCH", "/api/v1/notes/note1", `{"title":"Про собак","text":"Собаки лают."}`, "Authorization", "Bearer tok")
	if got := decode[noteBody](t, w); w.Code != http.StatusOK || got.Title != "Про собак" || got.EditedAt == nil ||
		h.notes.editedID != "note1" || h.notes.edited != [2]string{"Про собак", "Собаки лают."} {
		t.Fatalf("%d %+v", w.Code, got)
	}

	h = newHarness(rest.Config{})
	h.notes.err = failure.New(failure.PermissionDenied, "permission denied")
	if w = h.do("PATCH", "/api/v1/notes/note1", `{"title":"Т","text":"т"}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("a stranger edits: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("PATCH", "/api/v1/notes/note1", `{"title":"Т","text":"т"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("edit without token: %d", w.Code)
	}

	h = newHarness(rest.Config{})
	if w = h.do("DELETE", "/api/v1/notes/note1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.notes.deleted != "note1" || h.notes.token != "tok" {
		t.Fatalf("delete: %d, deleted %q", w.Code, h.notes.deleted)
	}
	if w = newHarness(rest.Config{}).do("DELETE", "/api/v1/notes/note1", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("delete without token: %d", w.Code)
	}
}

func TestNoteComments(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/notes/note1/comments?cursor=nc0&limit=2", "")
	got := decode[struct {
		Comments []struct {
			ID      string         `json:"id"`
			Author  map[string]any `json:"author"`
			Text    string         `json:"text"`
			TextCut bool           `json:"textCut"`
		} `json:"comments"`
		NextCursor string `json:"nextCursor"`
		Total      int    `json:"total"`
	}](t, w)

	// The comment of a deleted page is left out, still counted.
	if w.Code != http.StatusOK || len(got.Comments) != 1 || got.Total != 2 || got.NextCursor != "nc2" || h.notes.cursor != "nc0" || h.notes.limit != 2 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if c := got.Comments[0]; c.ID != "nc1" || c.Author["nickname"] != "bob" || c.Text != "мяу" || !c.TextCut {
		t.Fatalf("comment = %+v", c)
	}

	w = h.do("GET", "/api/v1/note-comments/nc1/text", "")
	if text := decode[map[string]string](t, w); w.Code != http.StatusOK || text["text"] != "мяу, целиком" {
		t.Fatalf("the whole text: %d %v", w.Code, text)
	}
}

func TestCreateNoteComment(t *testing.T) {
	t.Parallel()

	completed := func() harness {
		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		return h
	}

	h := completed()
	w := h.do("POST", "/api/v1/notes/note1/comments", `{"text":"мур"}`, "Authorization", "Bearer tok")
	got := decode[struct {
		ID     string         `json:"id"`
		Author map[string]any `json:"author"`
		Mine   bool           `json:"mine"`
	}](t, w)
	// Under her own note nobody asks about a blacklist.
	if w.Code != http.StatusCreated || got.ID != "nc9" || got.Author["nickname"] != "ann" || !got.Mine || h.notes.commented != "note1/мур" || h.users.relationWith != "" {
		t.Fatalf("%d %+v, commented %q, relation with %q", w.Code, got, h.notes.commented, h.users.relationWith)
	}

	h = newHarness(rest.Config{})
	w = h.do("POST", "/api/v1/notes/note1/comments", `{"text":"мур"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusBadRequest || decode[errorBody](t, w).Error.Message != "fill in your profile before commenting" || h.notes.commented != "" {
		t.Fatalf("without a profile: %d %s", w.Code, w.Body)
	}

	// Under the note of another a blacklist either way closes comments.
	h = completed()
	if w = h.do("POST", "/api/v1/notes/note3/comments", `{"text":"мур"}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated || h.users.relationWith != "u2" {
		t.Fatalf("under the note of another: %d, relation with %q", w.Code, h.users.relationWith)
	}

	h = completed()
	h.users.relation = model.Relation{BlockedMe: true}
	w = h.do("POST", "/api/v1/notes/note3/comments", `{"text":"мур"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusForbidden || decode[errorBody](t, w).Error.Message != "you cannot comment on this note" || h.notes.commented != "" {
		t.Fatalf("blacklisted: %d %s", w.Code, w.Body)
	}

	h = completed()
	h.users.hiddenNotes = true
	if w = h.do("POST", "/api/v1/notes/note1/comments", `{"text":"мур"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound || h.notes.commented != "" {
		t.Fatalf("hidden notes: %d", w.Code)
	}

	if w = completed().do("POST", "/api/v1/notes/note1/comments", `{"text":"мур"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}

	h = completed()
	if w = h.do("DELETE", "/api/v1/note-comments/nc1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.notes.deletedComment != "nc1" {
		t.Fatalf("delete a comment: %d, deleted %q", w.Code, h.notes.deletedComment)
	}
	if w = completed().do("DELETE", "/api/v1/note-comments/nc1", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("delete a comment without token: %d", w.Code)
	}
}
