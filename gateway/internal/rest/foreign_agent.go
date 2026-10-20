package rest

import "net/http"

// markForeignAgent puts on a page the orange plaque of a foreign agent,
// openvk's "user-alert" (PUT), or takes it (DELETE): only an
// administrator of the site may, the user service tells.
func (a *api) markForeignAgent(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// As with the checkmark, the profile is read anonymously only for its
	// ID: whoever the page is hidden from, the administrator marks it.
	page, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.users.SetForeignAgent(r.Context(), token, page.ID, r.Method == http.MethodPut); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
