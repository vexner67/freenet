package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
)

// banRequest blocks a page: why, and until when (RFC 3339), empty for good.
type banRequest struct {
	Reason string `json:"reason"`
	Until  string `json:"until"`
}

// ban blocks a page, openvk's "Заблокировать пользователя": PUT
// /api/v1/users/{nickname}/ban, only an administrator of the site may, the
// user service tells.
func (a *api) ban(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req banRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	var until time.Time
	if req.Until != "" {
		if until, err = time.Parse(time.RFC3339, req.Until); err != nil {
			a.writeError(w, r, failure.New(failure.InvalidArgument, "invalid until"))
			return
		}
	}

	id, err := a.pageID(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.users.Ban(r.Context(), token, id, req.Reason, until); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unban lifts the block of a page: DELETE /api/v1/users/{nickname}/ban.
func (a *api) unban(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	id, err := a.pageID(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.users.Unban(r.Context(), token, id); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// pageID is the ID of the page the path names, read anonymously: an
// administrator acts on a page whoever it is hidden from.
func (a *api) pageID(r *http.Request) (string, error) {
	page, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		return "", err
	}

	return page.ID, nil
}
