package rest

import (
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// uploadClubBackdrop takes the raw image as the request body and shows it
// on one side, left or right, of the club's page, as openvk's "Фон
// страницы", where the other side stays as it is. Only its admins may.
// The photo the club no longer shows goes only once it shows the new one.
func (a *api) uploadClubBackdrop(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	side := r.PathValue("side")
	if side != "left" && side != "right" {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "invalid backdrop side"))
		return
	}

	c, err := a.managedClub(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	image, err := readImage(w, r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	photoID, err := a.media.UploadClubPhoto(r.Context(), token, c.Club.ID, image)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	v, err := a.clubs.SetBackdrop(r.Context(), token, c.Club.ID, side, photoID)
	if err != nil {
		a.cleanUpClubPhotos(r, token, c.Club)
		a.writeError(w, r, err)
		return
	}

	a.cleanUpClubPhotos(r, token, v.Club)
	a.writeClub(w, r, http.StatusOK, v)
}

// deleteClubBackdrop takes both sides away, as openvk's "Удалить фон".
func (a *api) deleteClubBackdrop(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.managedClub(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	v := c
	for _, side := range []string{"left", "right"} {
		if v, err = a.clubs.SetBackdrop(r.Context(), token, c.Club.ID, side, ""); err != nil {
			a.writeError(w, r, err)
			return
		}
	}

	a.cleanUpClubPhotos(r, token, v.Club)
	a.writeClub(w, r, http.StatusOK, v)
}

// cleanUpClubPhotos deletes the photos c no longer shows. A failure only
// leaves unused files behind, so it is logged, not returned.
func (a *api) cleanUpClubPhotos(r *http.Request, token string, c model.Club) {
	var keep []string
	for _, id := range []string{c.BackdropLeft, c.BackdropRight} {
		if id != "" {
			keep = append(keep, id)
		}
	}

	if err := a.media.DeleteClubPhotos(r.Context(), token, c.ID, keep); err != nil {
		a.logger.WarnContext(r.Context(), "old club photos left behind", "error", err)
	}
}
