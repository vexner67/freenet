package rest

import "net/http"

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
