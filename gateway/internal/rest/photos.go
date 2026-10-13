package rest

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
)

// releaseTimeout bounds giving claimed photos back after the request.
const releaseTimeout = 5 * time.Second

// releasePauses are the waits before each attempt to give photos back:
// media stumbling once should not cost the caller the uploads.
var releasePauses = [...]time.Duration{0, 100 * time.Millisecond, 400 * time.Millisecond}

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

// claimPhotos takes the caller's uploads for the post, comment or message
// that is about to show them: the services that keep those store IDs and
// cannot tell whose photos they are, nor whether another post shows them
// already. A photo belongs to one post, which deletes the file when it is
// deleted itself, so a photo that is not the caller's upload, or that
// something has taken, refuses them all.
func (a *api) claimPhotos(r *http.Request, token string, photoIDs []string) error {
	if len(photoIDs) == 0 {
		return nil
	}

	claimed, err := a.media.ClaimPhotos(r.Context(), token, photoIDs)
	if err != nil {
		return err
	}

	if !claimed {
		return failure.New(failure.InvalidArgument, "photo not found")
	}

	return nil
}

// releasePhotos gives claimed photos back when what they were claimed for
// was refused, so that the next attempt finds them. Only a refusal: after
// a failure that does not say whether the post was written, the photos
// stay claimed. A release that keeps failing leaves them so too, until
// media lets the claim go (PHOTO_CLAIM_TTL).
func (a *api) releasePhotos(r *http.Request, token string, photoIDs []string, refused error) {
	if len(photoIDs) == 0 {
		return
	}

	switch failure.From(refused).Kind {
	case failure.InvalidArgument, failure.PermissionDenied, failure.NotFound, failure.Conflict, failure.TooManyRequests:
	default:
		return
	}

	// The client may be gone already; the photos go back all the same.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), releaseTimeout)
	defer cancel()

	var err error
	for _, pause := range releasePauses {
		if !wait(ctx, pause) {
			break
		}

		if err = a.media.ReleasePhotos(ctx, token, photoIDs); err == nil {
			return
		}
	}

	a.logger.WarnContext(ctx, "claimed photos not released", "error", err)
}

// wait pauses, telling whether it did so to the end.
func wait(ctx context.Context, pause time.Duration) bool {
	if pause <= 0 {
		return ctx.Err() == nil
	}

	timer := time.NewTimer(pause)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// uploadClaimed uploads a photo the gateway itself puts somewhere (a post
// about a new avatar, an album, a backdrop), taken at once: once its ID
// is shown, the caller cannot attach it to a post of theirs as well.
func (a *api) uploadClaimed(r *http.Request, token string, image []byte) (string, error) {
	photoID, err := a.media.UploadPhoto(r.Context(), token, image)
	if err != nil {
		return "", err
	}

	if err = a.claimPhotos(r, token, []string{photoID}); err != nil {
		return "", err
	}

	return photoID, nil
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
