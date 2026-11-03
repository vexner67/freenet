package rest

import (
	"net/http"
)

type callSignalRequest struct {
	Sealed string `json:"sealed"`
}

type callReportRequest struct {
	Seconds int `json:"seconds"`
}

type callServersJSON struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
}

// callPeer is the page of the path a call goes to: only someone the
// caller may write to, as a message.
func (a *api) callPeer(r *http.Request, token string) (string, error) {
	peer, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		return "", err
	}

	terms, _, err := a.messageTerms(r, token, peer.ID)
	if err != nil {
		return "", err
	}

	if !terms.Allowed {
		return "", refusal(terms)
	}

	return peer.ID, nil
}

// dialogCall hands a sealed signal of a call to the peer.
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

	peerID, err := a.callPeer(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.messages.CallSignal(r.Context(), token, peerID, req.Sealed); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// reportCall is the end of a call the caller made to the peer: seconds
// they talked, 0 when the peer did not answer; both hear of it in
// "Уведомления".
func (a *api) reportCall(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req callReportRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	peerID, err := a.callPeer(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.messages.ReportCall(r.Context(), token, peerID, req.Seconds); err != nil {
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
