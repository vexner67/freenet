package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// photoCommentJSON is a comment under a photo; mine: the viewer wrote it.
type photoCommentJSON struct {
	ID        string     `json:"id"`
	Author    authorJSON `json:"author"`
	Text      string     `json:"text"`
	CreatedAt string     `json:"createdAt"`
	Mine      bool       `json:"mine"`
}

type photoCommentPageResponse struct {
	Comments   []photoCommentJSON `json:"comments"`
	NextCursor string             `json:"nextCursor"`
	Total      int                `json:"total"`
}

type photoCommentRequest struct {
	Text string `json:"text"`
}

// visiblePhoto is the photo of the path, as getAlbumPhoto finds it: not
// found when its uploader's page is deleted or its album hidden from the
// caller.
func (a *api) visiblePhoto(r *http.Request, token string) (model.AlbumPhoto, error) {
	photo, album, err := a.albums.Photo(r.Context(), token, r.PathValue("id"))
	if err != nil {
		return model.AlbumPhoto{}, err
	}

	people, err := a.people(r, []string{photo.AuthorID})
	if err != nil {
		return model.AlbumPhoto{}, err
	}

	if author, ok := people[photo.AuthorID]; !ok || author.Deactivated {
		return model.AlbumPhoto{}, failure.New(failure.NotFound, "photo not found")
	}

	if _, err = a.albumOwner(r, token, album.OwnerID); err != nil {
		return model.AlbumPhoto{}, err
	}

	return photo, nil
}

func (a *api) likeAlbumPhoto(w http.ResponseWriter, r *http.Request) {
	a.setPhotoLike(w, r, true)
}

func (a *api) unlikeAlbumPhoto(w http.ResponseWriter, r *http.Request) {
	a.setPhotoLike(w, r, false)
}

func (a *api) setPhotoLike(w http.ResponseWriter, r *http.Request, liked bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.visiblePhoto(r, token); err != nil {
		a.writeError(w, r, err)
		return
	}

	set := a.albums.UnlikePhoto
	if liked {
		set = a.albums.LikePhoto
	}

	likes, err := set(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, likeResponse{Likes: likes, Liked: liked})
}

// listAlbumPhotoLikers is "Понравилось N людям" of a photo.
func (a *api) listAlbumPhotoLikers(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.visiblePhoto(r, token); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.listLikers(w, r, a.albums.PhotoLikers)
}

// listPhotoComments is the comments under a photo, oldest first; those of
// deleted pages are left out, still counted.
func (a *api) listPhotoComments(w http.ResponseWriter, r *http.Request) {
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

	if _, err = a.visiblePhoto(r, token); err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.albums.PhotoComments(r.Context(), token, r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
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

	resp := photoCommentPageResponse{
		Comments:   make([]photoCommentJSON, 0, len(page.Comments)),
		NextCursor: page.NextCursor,
		Total:      page.Total,
	}
	for _, c := range page.Comments {
		if author, ok := people[c.AuthorID]; ok && !author.Deactivated {
			resp.Comments = append(resp.Comments, toPhotoCommentJSON(c, author))
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// createPhotoComment needs a filled-in profile, as commenting a post
// does, and no blacklist between the caller and who uploaded the photo.
func (a *api) createPhotoComment(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req photoCommentRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.completedMe(r, token, "fill in your profile before commenting")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	photo, err := a.visiblePhoto(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if photo.AuthorID != me.ID {
		rel, err := a.users.Relation(r.Context(), token, photo.AuthorID)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		if rel.Blocked() {
			a.writeError(w, r, failure.New(failure.PermissionDenied, "you cannot comment on this photo"))
			return
		}
	}

	c, err := a.albums.CommentPhoto(r.Context(), token, photo.ID, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toPhotoCommentJSON(c, authorOf(me.ID, me.AvatarID, me.Profile)))
}

func (a *api) deletePhotoComment(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.albums.DeletePhotoComment(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// shareAlbumPhoto is "Поделиться" under a photo: a post sharing it.
func (a *api) shareAlbumPhoto(w http.ResponseWriter, r *http.Request) {
	a.share(w, r, func(token string) (string, string, error) {
		photo, err := a.visiblePhoto(r, token)
		return "", photo.ID, err
	})
}

func toPhotoCommentJSON(c model.PhotoComment, author model.Author) photoCommentJSON {
	return photoCommentJSON{
		ID:        c.ID,
		Author:    toAuthorJSON(author),
		Text:      c.Text,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		Mine:      c.Mine,
	}
}
