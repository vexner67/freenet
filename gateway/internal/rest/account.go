package rest

import (
	"net/http"
)

type deactivateRequest struct {
	Reason string `json:"reason"`
}

// deactivate deletes the caller's page ("Удаление страницы" on openvk).
// The client refreshes its tokens afterwards: those of a deleted page are
// good only to see and restore it.
func (a *api) deactivate(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req deactivateRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	u, err := a.users.Deactivate(r.Context(), token, req.Reason)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeUser(w, r, u)
}

// reactivate restores the caller's deleted page; the client refreshes its
// tokens afterwards.
func (a *api) reactivate(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	u, err := a.users.Reactivate(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeUser(w, r, u)
}
