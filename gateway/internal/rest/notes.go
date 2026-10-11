package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type noteRequest struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type noteCommentRequest struct {
	Text string `json:"text"`
}

// noteJSON is a note of a person; mine: the viewer wrote it.
type noteJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	// TextCut: text is only its beginning, as the list carries it; the
	// whole is at /notes/{id}.
	TextCut   bool   `json:"textCut,omitempty"`
	Comments  int    `json:"comments"`
	CreatedAt string `json:"createdAt"`
	// EditedAt is null for a note never changed.
	EditedAt *string `json:"editedAt"`
	Mine     bool    `json:"mine"`
}

type notePageResponse struct {
	Author     authorJSON `json:"author"`
	Notes      []noteJSON `json:"notes"`
	NextCursor string     `json:"nextCursor"`
	Total      int        `json:"total"`
}

type noteResponse struct {
	Author authorJSON `json:"author"`
	Note   noteJSON   `json:"note"`
}

// noteCommentJSON is a comment under a note; mine: the viewer wrote it.
type noteCommentJSON struct {
	ID     string     `json:"id"`
	Author authorJSON `json:"author"`
	Text   string     `json:"text"`
	// TextCut: the whole is at /note-comments/{id}/text.
	TextCut   bool   `json:"textCut,omitempty"`
	CreatedAt string `json:"createdAt"`
	Mine      bool   `json:"mine"`
}

type noteCommentPageResponse struct {
	Comments   []noteCommentJSON `json:"comments"`
	NextCursor string            `json:"nextCursor"`
	Total      int               `json:"total"`
}

func toNoteJSON(n model.Note) noteJSON {
	resp := noteJSON{
		ID:        n.ID,
		Title:     n.Title,
		Text:      n.Text,
		TextCut:   n.TextCut,
		Comments:  n.Comments,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
		Mine:      n.Mine,
	}
	if !n.EditedAt.IsZero() {
		edited := n.EditedAt.UTC().Format(time.RFC3339)
		resp.EditedAt = &edited
	}

	return resp
}

func toNoteCommentJSON(c model.NoteComment, author model.Author) noteCommentJSON {
	return noteCommentJSON{
		ID:        c.ID,
		Author:    toAuthorJSON(author),
		Text:      c.Text,
		TextCut:   c.TextCut,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		Mine:      c.Mine,
	}
}

// listUserNotes is "Заметки" of a person, public, as "Кому видно мои
// заметки" allows; those of a deleted page are gone, as its wall.
func (a *api) listUserNotes(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	owner, err := a.visiblePage(r, token, notesPart)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.notes.List(r.Context(), token, owner.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := notePageResponse{
		Author: toAuthorJSON(authorOfProfile(owner)),
		Notes:  make([]noteJSON, 0, len(page.Notes)), NextCursor: page.NextCursor, Total: page.Total,
	}
	for _, n := range page.Notes {
		resp.Notes = append(resp.Notes, toNoteJSON(n))
	}

	writeJSON(w, http.StatusOK, resp)
}

// createNote is "Добавить запись" of the notes; a filled-in profile is
// needed, as to post: notes live at the address of their author.
func (a *api) createNote(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req noteRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.completedMe(r, token, "fill in your profile before writing notes"); err != nil {
		a.writeError(w, r, err)
		return
	}

	note, err := a.notes.Create(r.Context(), token, req.Title, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toNoteJSON(note))
}

// visibleNote is the note id with its author as the caller, anonymous
// without token, sees it: not found when the author's page is deleted or
// hidden, or the notes are hidden from the caller ("Кому видно мои
// заметки").
func (a *api) visibleNote(r *http.Request, token, id string) (model.Note, model.Author, error) {
	note, err := a.notes.Get(r.Context(), token, id)
	if err != nil {
		return model.Note{}, model.Author{}, err
	}

	notFound := failure.New(failure.NotFound, "note not found")

	people, err := a.people(r, []string{note.AuthorID})
	if err != nil {
		return model.Note{}, model.Author{}, err
	}

	author, ok := people[note.AuthorID]
	if !ok || author.Deactivated {
		return model.Note{}, model.Author{}, notFound
	}

	access, err := a.users.Access(r.Context(), token, note.AuthorID)
	if err != nil {
		return model.Note{}, model.Author{}, err
	}

	if !access.Page || !access.Notes {
		return model.Note{}, model.Author{}, notFound
	}

	return note, author, nil
}

// getNote is a note for its page, whole, with who wrote it.
func (a *api) getNote(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	note, author, err := a.visibleNote(r, token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, noteResponse{Author: toAuthorJSON(author), Note: toNoteJSON(note)})
}

// editNote is "Редактировать заметку": only its author, which the post
// service checks.
func (a *api) editNote(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req noteRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	note, err := a.notes.Edit(r.Context(), token, r.PathValue("id"), req.Title, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toNoteJSON(note))
}

func (a *api) deleteNote(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.notes.Delete(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listNoteComments is the comments under a note, oldest first; those of
// deleted pages are left out, still counted.
func (a *api) listNoteComments(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	note, _, err := a.visibleNote(r, token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.notes.Comments(r.Context(), token, note.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(page.Comments))
	for _, c := range page.Comments {
		ids = append(ids, c.AuthorID)
	}

	people, err := a.people(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := noteCommentPageResponse{
		Comments:   make([]noteCommentJSON, 0, len(page.Comments)),
		NextCursor: page.NextCursor,
		Total:      page.Total,
	}
	for _, c := range page.Comments {
		if author, ok := people[c.AuthorID]; ok && !author.Deactivated {
			resp.Comments = append(resp.Comments, toNoteCommentJSON(c, author))
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// createNoteComment needs a filled-in profile, as commenting a post
// does, and no blacklist between the caller and the author of the note.
func (a *api) createNoteComment(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req noteCommentRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.completedMe(r, token, "fill in your profile before commenting")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	note, _, err := a.visibleNote(r, token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if note.AuthorID != me.ID {
		rel, err := a.users.Relation(r.Context(), token, note.AuthorID)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		if rel.Blocked() {
			a.writeError(w, r, failure.New(failure.PermissionDenied, "you cannot comment on this note"))
			return
		}
	}

	c, err := a.notes.Comment(r.Context(), token, note.ID, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toNoteCommentJSON(c, authorOf(me.ID, me.AvatarID, me.Profile)))
}

// deleteNoteComment: its author or the author of the note, which the
// post service checks.
func (a *api) deleteNoteComment(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.notes.DeleteComment(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// noteCommentText is what a comment under a note says, whole: the list
// carries its beginning. A comment of a note the caller is not shown is
// not found.
func (a *api) noteCommentText(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.notes.CommentByID(r.Context(), r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, _, err = a.visibleNote(r, token, c.NoteID); err != nil {
		if failure.From(err).Kind == failure.NotFound {
			err = failure.New(failure.NotFound, "comment not found")
		}
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, textResponse{Text: c.Text})
}
