package rest

import (
	"context"
	"net/http"
)

// relationJSON is who of the viewer and the user blacklisted the other;
// both false for anonymous viewers.
type relationJSON struct {
	BlockedByMe bool `json:"blockedByMe"`
	BlockedMe   bool `json:"blockedMe"`
}

// listBlacklist is the caller's blacklist ("Чёрный список"), newest first.
func (a *api) listBlacklist(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.users.Blacklist(r.Context(), token, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := userPageResponse{
		Users:      make([]publicProfileResponse, 0, len(page.Users)),
		NextCursor: page.NextCursor,
		Total:      page.Total,
	}

	for _, u := range page.Users {
		resp.Users = append(resp.Users, toPublicProfileResponse(u))
	}

	writeJSON(w, http.StatusOK, resp)
}

// block blacklists the user with the nickname; it drops the follows
// between the two. Idempotent, as unblock is.
func (a *api) block(w http.ResponseWriter, r *http.Request) {
	a.changeBlock(w, r, a.users.Block)
}

func (a *api) unblock(w http.ResponseWriter, r *http.Request) {
	a.changeBlock(w, r, a.users.Unblock)
}

func (a *api) changeBlock(w http.ResponseWriter, r *http.Request, change func(ctx context.Context, accessToken, userID string) error) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	target, err := a.users.GetProfile(r.Context(), r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = change(r.Context(), token, target.ID); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
