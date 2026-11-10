package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/vexner67/freenet/pkg/errs"
)

// liveKitProxy exposes the WebSocket signaling endpoints only. The Twirp
// administration API is never routed to the public origin.
func liveKitProxy(raw string, rooms Rooms) (http.Handler, error) {
	target, err := url.Parse(raw)
	if err != nil {
		return nil, errs.Wrap("url.Parse", err)
	}
	if target.Host == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, errors.New("invalid LiveKit URL")
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{ResponseHeaderTimeout: 10 * time.Second, IdleConnTimeout: time.Minute}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("access_token")
		if token == "" {
			http.Error(w, "missing media token", http.StatusUnauthorized)
			return
		}
		if rooms == nil {
			http.Error(w, "rooms unavailable", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		err := rooms.AuthorizeMedia(ctx, token)
		cancel()
		if err != nil {
			http.Error(w, "media access denied", http.StatusForbidden)
			return
		}

		rc := http.NewResponseController(w)
		if err := rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			http.Error(w, "proxy unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			http.Error(w, "proxy unavailable", http.StatusServiceUnavailable)
			return
		}
		proxy.ServeHTTP(w, r)
	}), nil
}
