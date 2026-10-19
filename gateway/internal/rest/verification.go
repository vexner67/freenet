package rest

import (
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/failure"
)

// verify gives a page the checkmark of a confirmed one, openvk's
// "Верификация" (PUT), or takes it (DELETE): only an administrator of the
// site may, the user service tells.
func (a *api) verify(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// Whoever the page is hidden from, the administrator gives it the
	// checkmark: the profile is read anonymously only for its ID.
	page, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.users.SetVerified(r.Context(), token, page.ID, r.Method == http.MethodPut); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// verifyClub gives a club the checkmark (PUT) or takes it (DELETE). The
// club service knows no administrators of the site, so the gateway asks
// the user service whether the caller is one before it calls.
func (a *api) verifyClub(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !me.Admin {
		a.writeError(w, r, failure.New(failure.PermissionDenied, "permission denied"))
		return
	}

	// A closed club is verified as any: it is read only for its ID.
	c, err := a.clubs.Get(r.Context(), "", r.PathValue("ref"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.clubs.SetVerified(r.Context(), c.Club.ID, r.Method == http.MethodPut); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
