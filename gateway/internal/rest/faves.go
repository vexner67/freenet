package rest

import (
	"context"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// listFavePosts is openvk's "Закладки": the posts the caller likes, newest
// like first, shown as a wall shows them. Those of deleted pages are left
// out, but counted.
func (a *api) listFavePosts(w http.ResponseWriter, r *http.Request) {
	a.writePosts(w, r, a.faves.Posts)
}

// listOwnPosts is openvk's "Написанные мной записи": what the caller
// wrote, on their wall and on others'.
func (a *api) listOwnPosts(w http.ResponseWriter, r *http.Request) {
	a.writePosts(w, r, a.faves.OwnPosts)
}

func (a *api) writePosts(
	w http.ResponseWriter, r *http.Request,
	list func(ctx context.Context, accessToken, cursor string, limit int) (model.PostPage, error),
) {
	token, limit, err := ownList(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := list(r.Context(), token, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown, err := a.withPeople(r, page.Posts)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, postPageResponse{Posts: shown, NextCursor: page.NextCursor, Total: page.Total})
}

// ownList is who asks for a list of their own, and how much of it.
func ownList(r *http.Request) (token string, limit int, err error) {
	if token, err = bearerToken(r); err != nil {
		return "", 0, err
	}

	if limit, err = pageLimit(r); err != nil {
		return "", 0, err
	}

	return token, limit, nil
}

// listFaveComments are the comments the caller likes, each with the ID of
// its post. A comment is left out, but counted, when its author, the
// author of its post or the owner of the wall deleted the page.
func (a *api) listFaveComments(w http.ResponseWriter, r *http.Request) {
	token, limit, err := ownList(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.faves.Comments(r.Context(), token, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	postIDs := make([]string, 0, len(page.Comments))
	for _, c := range page.Comments {
		postIDs = append(postIDs, c.PostID)
	}

	// The posts tell the walls: a comment for a community shows it.
	found, err := a.posts.GetMany(r.Context(), token, unique(postIDs))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	posts := make(map[string]model.Post, len(found))
	ids := make([]string, 0, len(page.Comments)+2*len(found))
	for _, p := range found {
		posts[p.ID] = p
		ids = append(ids, p.Author.ID, p.Wall.ID)
	}
	for _, c := range page.Comments {
		ids = append(ids, c.Author.ID)
	}

	people, err := a.authors(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := commentPageResponse{
		Comments:   make([]commentResponse, 0, len(page.Comments)),
		NextCursor: page.NextCursor,
		Total:      page.Total,
	}

	for _, c := range page.Comments {
		p, ok := posts[c.PostID]
		if !ok {
			continue
		}

		wallID := p.Wall.ID
		if !withProfiles(&p, people) {
			continue
		}

		shown, ok := people[c.Author.ID]
		if c.FromWall {
			shown, ok = people[wallID]
		}
		if !ok {
			continue
		}

		resp.Comments = append(resp.Comments, toCommentResponse(c, shown))
	}

	writeJSON(w, http.StatusOK, resp)
}

// listFavePhotos are the photos of albums the caller likes. A photo is
// left out, but counted, when its uploader deleted the page or the owner
// of its album no longer shows the caller their photos.
func (a *api) listFavePhotos(w http.ResponseWriter, r *http.Request) {
	token, limit, err := ownList(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.faves.Photos(r.Context(), token, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	uploaders := make([]string, 0, len(page.Photos))
	for _, p := range page.Photos {
		uploaders = append(uploaders, p.AuthorID)
	}

	people, err := a.people(r, uploaders)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := albumPhotoPageResponse{
		Photos:     make([]albumPhotoJSON, 0, len(page.Photos)),
		NextCursor: page.NextCursor,
		Total:      page.Total,
	}

	// Each owner is asked once.
	visible := make(map[string]bool)

	for _, p := range page.Photos {
		if uploader, ok := people[p.AuthorID]; !ok || uploader.Deactivated {
			continue
		}

		shown, asked := visible[p.OwnerID]
		if !asked {
			_, err = a.albumOwner(r, token, p.OwnerID)
			if err != nil && failure.From(err).Kind != failure.NotFound {
				a.writeError(w, r, err)
				return
			}

			shown = err == nil
			visible[p.OwnerID] = shown
		}

		if shown {
			resp.Photos = append(resp.Photos, toAlbumPhotoJSON(p.AlbumPhoto))
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
