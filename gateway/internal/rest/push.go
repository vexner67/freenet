package rest

import (
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

type pushKeyResponse struct {
	// PublicKey is the applicationServerKey to subscribe with; null when
	// the site does not push.
	PublicKey *string `json:"publicKey"`
}

// pushSubscriptionRequest is PushSubscription.toJSON() of the browser.
type pushSubscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	// ExpirationTime comes with it and is not used: a browser whose
	// subscription ended is forgotten on the first push it refuses.
	ExpirationTime *float64 `json:"expirationTime"`
}

type pushEndpointRequest struct {
	Endpoint string `json:"endpoint"`
}

// getPushKey tells browsers what to subscribe to pushes with; anyone may
// ask.
func (a *api) getPushKey(w http.ResponseWriter, r *http.Request) {
	key, err := a.notifications.PushKey(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := pushKeyResponse{}
	if key != "" {
		resp.PublicKey = &key
	}

	writeJSON(w, http.StatusOK, resp)
}

// subscribePush lets the caller's browser take their pushes; the browser
// pushes to whoever subscribed it last.
func (a *api) subscribePush(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req pushSubscriptionRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	s := model.PushSubscription{Endpoint: req.Endpoint, P256DH: req.Keys.P256DH, Auth: req.Keys.Auth}
	if err = a.notifications.SubscribePush(r.Context(), token, s); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unsubscribePush stops pushing to the caller's browser. The endpoint is
// in the body, not the URL: it is the browser's secret address.
func (a *api) unsubscribePush(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req pushEndpointRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.notifications.UnsubscribePush(r.Context(), token, req.Endpoint); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
