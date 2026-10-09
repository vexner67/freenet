package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type albumRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type albumPhotosRequest struct {
	Photos []newAlbumPhotoRequest `json:"photos"`
}

type newAlbumPhotoRequest struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type albumPhotoRequest struct {
	Description string `json:"description"`
}

type albumJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Photos      int    `json:"photos"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	// Cover is the first photo, null while there is none.
	Cover *photoJSON `json:"cover"`
	// Kind is "" for an album people make, "avatars" for "Фотографии со
	// страницы", which the site keeps: nobody renames, deletes or fills it
	// by hand.
	Kind string `json:"kind"`
}

// albumPhotoJSON is a photo with its URLs, as of a post, and what is
// told about it; mine: the viewer uploaded it.
type albumPhotoJSON struct {
	photoJSON
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
	Mine        bool   `json:"mine"`
}

// albumOwnerJSON is who albums belong to: a person or a community, with
// role, what the viewer is in the community ("" for a person).
type albumOwnerJSON struct {
	Owner authorJSON `json:"owner"`
	Role  string     `json:"role"`
}

type albumPageResponse struct {
	albumOwnerJSON
	Albums     []albumJSON `json:"albums"`
	NextCursor string      `json:"nextCursor"`
	Total      int         `json:"total"`
}

type albumResponse struct {
	albumOwnerJSON
	Album albumJSON `json:"album"`
}

type albumPhotoPageResponse struct {
	Photos     []albumPhotoJSON `json:"photos"`
	NextCursor string           `json:"nextCursor"`
	Total      int              `json:"total"`
}

type addedAlbumPhotosResponse struct {
	Photos []albumPhotoJSON `json:"photos"`
}

// albumPhotoResponse is a photo for its page: who uploaded it (author)
// and its album with whom that belongs to.
type albumPhotoResponse struct {
	albumOwnerJSON
	Photo  albumPhotoJSON `json:"photo"`
	Author authorJSON     `json:"author"`
	Album  albumJSON      `json:"album"`
}

func toAlbumJSON(a model.Album) albumJSON {
	resp := albumJSON{
		ID:          a.ID,
		Name:        a.Name,
		Description: a.Description,
		Photos:      a.Photos,
		CreatedAt:   a.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   a.UpdatedAt.UTC().Format(time.RFC3339),
		Kind:        a.Kind,
	}
	if a.Cover != nil {
		cover := toPhotoJSON(a.Cover.AuthorID, a.Cover.ID)
		resp.Cover = &cover
	}

	return resp
}

func toAlbumPhotoJSON(p model.AlbumPhoto) albumPhotoJSON {
	return albumPhotoJSON{
		photoJSON:   toPhotoJSON(p.AuthorID, p.ID),
		Description: p.Description,
		CreatedAt:   p.CreatedAt.UTC().Format(time.RFC3339),
		Mine:        p.Mine,
	}
}

func toAlbumsJSON(albums []model.Album) []albumJSON {
	out := make([]albumJSON, 0, len(albums))
	for _, a := range albums {
		out = append(out, toAlbumJSON(a))
	}

	return out
}

// listUserAlbums is "Альбомы" of a person, public, as "Кому видно мои
// фотографии" allows; those of a deleted page are gone, as its wall.
func (a *api) listUserAlbums(w http.ResponseWriter, r *http.Request) {
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

	owner, err := a.visiblePage(r, token, photosPart)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.albums.List(r.Context(), token, owner.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, albumPageResponse{
		albumOwnerJSON: albumOwnerJSON{Owner: toAuthorJSON(authorOfProfile(owner))},
		Albums:         toAlbumsJSON(page.Albums), NextCursor: page.NextCursor, Total: page.Total,
	})
}

// listClubAlbums is "Альбомы" of a community, public.
func (a *api) listClubAlbums(w http.ResponseWriter, r *http.Request) {
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

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.albums.List(r.Context(), token, c.Club.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, albumPageResponse{
		albumOwnerJSON: albumOwnerJSON{Owner: toAuthorJSON(clubAuthor(c.Club)), Role: c.Role},
		Albums:         toAlbumsJSON(page.Albums), NextCursor: page.NextCursor, Total: page.Total,
	})
}

// createAlbum is "Создать альбом" of one's own; a filled-in profile is
// needed, as to post: albums live at the address of their owner.
func (a *api) createAlbum(w http.ResponseWriter, r *http.Request) {
	a.newAlbum(w, r, false)
}

// createClubAlbum is "Создать альбом" of a community: its admins, which
// the post service checks.
func (a *api) createClubAlbum(w http.ResponseWriter, r *http.Request) {
	a.newAlbum(w, r, true)
}

func (a *api) newAlbum(w http.ResponseWriter, r *http.Request, ofClub bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req albumRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.completedMe(r, token, "fill in your profile before creating albums"); err != nil {
		a.writeError(w, r, err)
		return
	}

	na := model.NewAlbum{Name: req.Name, Description: req.Description}
	if ofClub {
		c, err := a.club(r, token)
		if err != nil {
			a.writeError(w, r, err)
			return
		}
		na.ClubID = c.Club.ID
	}

	album, err := a.albums.Create(r.Context(), token, na)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toAlbumJSON(album))
}

// albumOwner is whom an album belongs to as the viewer sees it: a person,
// or a community with the viewer's role in it. The albums of a deleted
// page are not found, as its wall is gone.
func (a *api) albumOwner(r *http.Request, token, ownerID string) (albumOwnerJSON, error) {
	found, err := a.authors(r, []string{ownerID})
	if err != nil {
		return albumOwnerJSON{}, err
	}

	owner, ok := found[ownerID]
	if !ok {
		return albumOwnerJSON{}, failure.New(failure.NotFound, "album not found")
	}

	if !owner.Club {
		// "Кому видно мои фотографии": the albums of a person the caller
		// may not see are not found.
		access, err := a.users.Access(r.Context(), token, ownerID)
		if err != nil {
			return albumOwnerJSON{}, err
		}
		if !access.Page || !access.Photos {
			return albumOwnerJSON{}, failure.New(failure.NotFound, "album not found")
		}

		return albumOwnerJSON{Owner: toAuthorJSON(owner)}, nil
	}

	c, err := a.clubs.Get(r.Context(), token, ownerID)
	if err != nil {
		return albumOwnerJSON{}, err
	}

	return albumOwnerJSON{Owner: toAuthorJSON(owner), Role: c.Role}, nil
}

// albumOf is the album of the path with whom it belongs to.
func (a *api) albumOf(r *http.Request, token string) (model.Album, albumOwnerJSON, error) {
	album, err := a.albums.Get(r.Context(), token, r.PathValue("id"))
	if err != nil {
		return model.Album{}, albumOwnerJSON{}, err
	}

	owner, err := a.albumOwner(r, token, album.OwnerID)
	if err != nil {
		return model.Album{}, albumOwnerJSON{}, err
	}

	return album, owner, nil
}

// getAlbum is an album with its owner, for its page; public.
func (a *api) getAlbum(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	album, owner, err := a.albumOf(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, albumResponse{albumOwnerJSON: owner, Album: toAlbumJSON(album)})
}

// editAlbum saves "Редактировать альбом"; the post service decides who
// may.
func (a *api) editAlbum(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req albumRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.albums.Edit(r.Context(), token, r.PathValue("id"), req.Name, req.Description); err != nil {
		a.writeError(w, r, err)
		return
	}

	// The album again, with its owner.
	a.getAlbum(w, r)
}

func (a *api) deleteAlbum(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.albums.Delete(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listAlbumPhotos is an album's photos, oldest first; public. The photos
// of people whose page is deleted are left out, but counted in total.
func (a *api) listAlbumPhotos(w http.ResponseWriter, r *http.Request) {
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

	album, _, err := a.albumOf(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.albums.Photos(r.Context(), token, album.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(page.Photos))
	for _, p := range page.Photos {
		ids = append(ids, p.AuthorID)
	}

	people, err := a.authors(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := albumPhotoPageResponse{Photos: make([]albumPhotoJSON, 0, len(page.Photos)), NextCursor: page.NextCursor, Total: page.Total}
	for _, p := range page.Photos {
		if _, ok := people[p.AuthorID]; ok {
			resp.Photos = append(resp.Photos, toAlbumPhotoJSON(p))
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// addAlbumPhotos puts photos the caller uploaded (POST /me/photos) in an
// album, "Завершить загрузку": the post service decides who may.
func (a *api) addAlbumPhotos(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req albumPhotosRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.completedMe(r, token, "fill in your profile before uploading photos"); err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(req.Photos))
	photos := make([]model.NewAlbumPhoto, 0, len(req.Photos))
	for _, p := range req.Photos {
		ids = append(ids, p.ID)
		photos = append(photos, model.NewAlbumPhoto{PhotoID: p.ID, Description: p.Description})
	}

	if err = a.checkOwnPhotos(r, token, ids); err != nil {
		a.writeError(w, r, err)
		return
	}

	added, err := a.albums.AddPhotos(r.Context(), token, r.PathValue("id"), photos)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := addedAlbumPhotosResponse{Photos: make([]albumPhotoJSON, 0, len(added))}
	for _, p := range added {
		resp.Photos = append(resp.Photos, toAlbumPhotoJSON(p))
	}

	writeJSON(w, http.StatusCreated, resp)
}

// getAlbumPhoto is a photo for its page, with who uploaded it and its
// album; public.
func (a *api) getAlbumPhoto(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	photo, album, err := a.albums.Photo(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	people, err := a.people(r, []string{photo.AuthorID})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	author, ok := people[photo.AuthorID]
	if !ok || author.Deactivated {
		a.writeError(w, r, failure.New(failure.NotFound, "photo not found"))
		return
	}

	owner, err := a.albumOwner(r, token, album.OwnerID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, albumPhotoResponse{
		albumOwnerJSON: owner, Photo: toAlbumPhotoJSON(photo), Author: toAuthorJSON(author), Album: toAlbumJSON(album),
	})
}

// editAlbumPhoto changes what is told about a photo: who uploaded it
// only, which the post service checks.
func (a *api) editAlbumPhoto(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req albumPhotoRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.albums.Describe(r.Context(), token, r.PathValue("id"), req.Description); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.getAlbumPhoto(w, r)
}

func (a *api) deleteAlbumPhoto(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.albums.DeletePhoto(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
