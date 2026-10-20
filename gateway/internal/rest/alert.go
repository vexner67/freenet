package rest

import "net/http"

// setAlert puts on a page the orange label of openvk ("user-alert") with
// the words of the body (PUT) or takes it (DELETE): only an administrator
// of the site may, the user service tells.
func (a *api) setAlert(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Alert string `json:"alert"`
	}
	if r.Method == http.MethodPut {
		if err = decode(w, r, &req); err != nil {
			a.writeError(w, r, err)
			return
		}
	}

	// As with the checkmark, the profile is read anonymously only for its
	// ID: whoever the page is hidden from, the administrator labels it.
	page, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.users.SetAlert(r.Context(), token, page.ID, req.Alert); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
