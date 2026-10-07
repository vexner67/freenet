package rest

import (
	"net/http"
	"net/url"

	"github.com/vexner67/freenet/gateway/internal/failure"
)

// photoJSON is a photo of a post with the URLs of both sizes. The photo ID
// in the path makes the URLs unique, so they are cached forever.
type photoJSON struct {
	ID    string `json:"id"`
	Large string `json:"large"`
	Small string `json:"small"`
}

func toPhotoJSON(ownerID, photoID string) photoJSON {
	base := "/media/photos/" + url.PathEscape(ownerID) + "/" + url.PathEscape(photoID) + "/"

	return photoJSON{
		ID:    photoID,
		Large: base + largeImageFile,
		Small: base + smallImageFile,
	}
}

func toPhotosJSON(ownerID string, photoIDs []string) []photoJSON {
	photos := make([]photoJSON, len(photoIDs))
	for i, id := range photoIDs {
		photos[i] = toPhotoJSON(ownerID, id)
	}

	return photos
}

// uploadPhoto takes the raw image as the request body and stores it for a
// post the caller is writing; the post refers to it by ID.
func (a *api) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	image, err := readImage(w, r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	photoID, err := a.media.UploadPhoto(r.Context(), token, image)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toPhotoJSON(me.ID, photoID))
}

// checkOwnPhotos makes sure a post shows only photos its author uploaded:
// the post service stores IDs and cannot tell whose photos they are.
func (a *api) checkOwnPhotos(r *http.Request, token string, photoIDs []string) error {
	if len(photoIDs) == 0 {
		return nil
	}

	own, err := a.media.OwnPhotos(r.Context(), token, photoIDs)
	if err != nil {
		return err
	}

	owned := make(map[string]bool, len(own))
	for _, id := range own {
		owned[id] = true
	}

	for _, id := range photoIDs {
		if !owned[id] {
			return failure.New(failure.InvalidArgument, "photo not found")
		}
	}

	return nil
}

// servePhoto is the image behind a photo URL.
func (a *api) servePhoto(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	if file != largeImageFile && file != smallImageFile {
		a.writeError(w, r, failure.New(failure.NotFound, "photo not found"))
		return
	}

	image, err := a.media.Photo(r.Context(), r.PathValue("userID"), r.PathValue("photoID"), file == smallImageFile)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	serveImage(w, image)
}
