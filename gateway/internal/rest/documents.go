package rest

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

const (
	// maxDocumentBytes matches what the media service accepts.
	maxDocumentBytes = 10 << 20
	// maxAttachedDocuments is how many documents a post or a message
	// attaches.
	maxAttachedDocuments = 10
	// documentBatch is how many documents the post service tells at once.
	documentBatch = 50
)

// documentJSON is a document as a list of "Мои Документы", a post or a
// message shows it; url downloads it.
type documentJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Ext       string `json:"ext"`
	Size      int64  `json:"size"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt"`
}

type documentPageResponse struct {
	Documents  []documentJSON `json:"documents"`
	NextCursor string         `json:"nextCursor"`
	Total      int            `json:"total"`
}

type renameDocumentRequest struct {
	Name string `json:"name"`
}

func toDocumentJSON(d model.Document) documentJSON {
	return documentJSON{
		ID:        d.ID,
		Name:      d.Name,
		Ext:       d.Ext,
		Size:      d.Size,
		URL:       "/media/documents/" + url.PathEscape(d.ID),
		CreatedAt: d.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// documentsJSON are the documents of ids that are still there, in order.
func documentsJSON(ids []string, known map[string]model.Document) []documentJSON {
	if len(ids) == 0 {
		return nil
	}

	docs := make([]documentJSON, 0, len(ids))
	for _, id := range ids {
		if d, ok := known[id]; ok {
			docs = append(docs, toDocumentJSON(d))
		}
	}

	return docs
}

// documentsOf tells the documents of ids by ID, a batch at a time; a
// deleted one is not there.
func (a *api) documentsOf(ctx context.Context, ids []string) (map[string]model.Document, error) {
	known := make(map[string]model.Document)
	if a.documents == nil {
		return known, nil
	}

	wanted := uniqueIDs(ids)
	for len(wanted) > 0 {
		n := min(len(wanted), documentBatch)

		found, err := a.documents.GetMany(ctx, wanted[:n])
		if err != nil {
			return nil, err
		}

		for _, d := range found {
			known[d.ID] = d
		}

		wanted = wanted[n:]
	}

	return known, nil
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	return unique
}

// checkDocuments makes sure the documents a post or a message is about to
// attach are the caller's: the services that keep them store IDs only.
func (a *api) checkDocuments(ctx context.Context, ownerID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	if len(ids) > maxAttachedDocuments {
		return failure.New(failure.InvalidArgument, "too many documents")
	}

	known, err := a.documentsOf(ctx, ids)
	if err != nil {
		return err
	}

	for _, id := range ids {
		if d, ok := known[id]; !ok || d.OwnerID != ownerID {
			return failure.New(failure.InvalidArgument, "document not found")
		}
	}

	return nil
}

// listDocuments is "Мои Документы": the caller's documents, newest first.
func (a *api) listDocuments(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.documents.List(r.Context(), token, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := documentPageResponse{Documents: []documentJSON{}, NextCursor: page.NextCursor, Total: page.Total}
	for _, d := range page.Documents {
		resp.Documents = append(resp.Documents, toDocumentJSON(d))
	}

	writeJSON(w, http.StatusOK, resp)
}

// uploadDocument takes the raw file as the request body and ?name= as
// the name of the file: the media service keeps the bytes, the post
// service what the document is. A file the post service refuses (its
// format, its name) is deleted from media again.
func (a *api) uploadDocument(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	data, err := readDocument(w, r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	id, err := a.media.UploadDocument(r.Context(), token, data)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	d, err := a.documents.Add(r.Context(), token, id, r.URL.Query().Get("name"), int64(len(data)))
	if err != nil {
		a.dropDocument(r, me.ID, id)
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toDocumentJSON(d))
}

func readDocument(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDocumentBytes))

	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return nil, failure.New(failure.InvalidArgument, "document is larger than 10 MB")
	}

	if err == nil && len(data) == 0 {
		return nil, failure.New(failure.InvalidArgument, "document is empty")
	}

	return data, err
}

// dropDocument deletes a file media kept for a document the post service
// refused; the client may be gone already.
func (a *api) dropDocument(r *http.Request, ownerID, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), releaseTimeout)
	defer cancel()

	if err := a.media.DeleteDocuments(ctx, ownerID, []string{id}); err != nil {
		a.logger.WarnContext(ctx, "refused document not deleted", "error", err)
	}
}

func (a *api) renameDocument(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req renameDocumentRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	d, err := a.documents.Rename(r.Context(), token, r.PathValue("id"), req.Name)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toDocumentJSON(d))
}

func (a *api) deleteDocument(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.documents.Delete(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// serveDocument downloads a document by its ID, as whoever sees what
// attaches it may: the ID is the secret, as with photos.
func (a *api) serveDocument(w http.ResponseWriter, r *http.Request) {
	known, err := a.documentsOf(r.Context(), []string{r.PathValue("id")})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	d, ok := known[r.PathValue("id")]
	if !ok {
		a.writeError(w, r, failure.New(failure.NotFound, "document not found"))
		return
	}

	data, err := a.media.Document(r.Context(), d.OwnerID, d.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName(d)}))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A deleted document goes away, so it is not cached for good.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}

// fileName is what a downloaded document is saved as: its name, with its
// format at the end.
func fileName(d model.Document) string {
	if strings.HasSuffix(strings.ToLower(d.Name), "."+d.Ext) {
		return d.Name
	}

	return d.Name + "." + d.Ext
}

// postDocuments are the documents the posts attach, by ID; none asked
// when no post attaches one.
func (a *api) postDocuments(r *http.Request, posts ...model.Post) (map[string]model.Document, error) {
	var ids []string
	for _, p := range posts {
		ids = append(ids, p.DocumentIDs...)
	}

	if len(ids) == 0 {
		return nil, nil
	}

	return a.documentsOf(r.Context(), ids)
}
