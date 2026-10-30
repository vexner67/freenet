package rest

import (
	"net/http"
)

type callSignalRequest struct {
	Sealed string `json:"sealed"`
}

type callServersJSON struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
}

// dialogCall hands a sealed signal of a call to the peer: only to someone
// the caller may write to, as a message.
func (a *api) dialogCall(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req callSignalRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	peer, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	terms, _, err := a.messageTerms(r, token, peer.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !terms.Allowed {
		a.writeError(w, r, refusal(terms))
		return
	}

	if err = a.messages.CallSignal(r.Context(), token, peer.ID, req.Sealed); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) callServers(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	servers, err := a.messages.CallServers(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	urls := servers.URLs
	if urls == nil {
		urls = []string{}
	}
	writeJSON(w, http.StatusOK, callServersJSON{URLs: urls, Username: servers.Username, Credential: servers.Credential})
}
