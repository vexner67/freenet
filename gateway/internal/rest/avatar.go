package rest

import (
	"errors"
	"io"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

const (
	// maxImageBytes matches what the media service accepts.
	maxImageBytes = 8 << 20

	// The files behind avatar and photo URLs.
	largeImageFile = "large.jpg"
	smallImageFile = "small.jpg"
)

// readImage reads an uploaded image, the raw request body.
func readImage(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	image, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImageBytes))

	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return nil, failure.New(failure.InvalidArgument, "image is larger than 8 MB")
	}

	return image, err
}

// serveImage writes a JPEG behind a URL that changes with the picture, so
// it can be cached for good.
func serveImage(w http.ResponseWriter, image []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(image)
}

// uploadAvatar takes the raw image as the request body and makes it the
// caller's photo. The old photo is deleted only after the profile points
// to the new one, so the profile never shows a missing picture. Unlike
// openvk, no post tells of the new photo (the owner of the site decided
// so).
func (a *api) uploadAvatar(w http.ResponseWriter, r *http.Request) {
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

	avatarID, err := a.media.UploadAvatar(r.Context(), token, image)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.SetAvatar(r.Context(), token, avatarID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.cleanUpAvatars(r, token, avatarID)
	a.keepInAvatarAlbum(r, token, "", image)
	a.writeUser(w, r, me)
}

// keepInAvatarAlbum puts the new photo of a page (clubID: of a club, ""
// for the caller's) in its "Фотографии со страницы", as openvk does: an
// image of its own, so that deleting it there leaves the avatar alone.
// The photo is changed already, so a failure is only logged.
func (a *api) keepInAvatarAlbum(r *http.Request, token, clubID string, image []byte) {
	photoID, err := a.uploadClaimed(r, token, image)
	if err == nil {
		err = a.albums.AddAvatarPhoto(r.Context(), token, clubID, photoID)
	}

	if err != nil {
		a.logger.WarnContext(r.Context(), "new photo left out of the avatar album", "error", err)
	}
}

func (a *api) deleteAvatar(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.SetAvatar(r.Context(), token, "")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.cleanUpAvatars(r, token, "")

	a.writeUser(w, r, me)
}

// cleanUpAvatars deletes photos no longer shown. A failure only leaves
// unused files behind, so it is logged, not returned.
func (a *api) cleanUpAvatars(r *http.Request, token, keep string) {
	if err := a.media.DeleteAvatars(r.Context(), token, keep); err != nil {
		a.logger.WarnContext(r.Context(), "old avatars left behind", "error", err)
	}
}

// serveAvatar is the image behind an avatar URL. The URL changes with every
// new photo, so the response can be cached for good.
func (a *api) serveAvatar(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	if file != largeImageFile && file != smallImageFile {
		a.writeError(w, r, failure.New(failure.NotFound, "avatar not found"))
		return
	}

	image, err := a.media.Avatar(r.Context(), r.PathValue("userID"), r.PathValue("avatarID"), file == smallImageFile)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	serveImage(w, image)
}

// searchUsers finds people by name, surname or nickname: GET
// /api/v1/users?q=, also by the filters openvk's page links to: hometown,
// city, film, show and book, and verified=true: only the pages with the
// checkmark.
func (a *api) searchUsers(w http.ResponseWriter, r *http.Request) {
	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	page, err := a.users.Search(r.Context(), model.UserQuery{
		Text: query.Get("q"), Hometown: query.Get("hometown"), City: query.Get("city"),
		Film: query.Get("film"), Show: query.Get("show"), Book: query.Get("book"),
		Verified: query.Get("verified") == "true",
	}, query.Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := userPageResponse{
		Users:      make([]publicProfileResponse, 0, len(page.Users)),
		NextCursor: page.NextCursor,
	}

	for _, u := range page.Users {
		resp.Users = append(resp.Users, toPublicProfileResponse(u))
	}

	writeJSON(w, http.StatusOK, resp)
}
