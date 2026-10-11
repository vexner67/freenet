package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// client is where the request comes from: a session remembers it.
func (a *api) client(r *http.Request) model.Client {
	return model.Client{
		IP:        clientIP(r, a.trustForwardedFor),
		UserAgent: r.UserAgent(),
	}
}

type sessionResponse struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"userAgent"`
	Current    bool      `json:"current"`
}

type sessionsResponse struct {
	Sessions []sessionResponse `json:"sessions"`
}

type endedSessionsResponse struct {
	Ended int `json:"ended"`
}

// listSessions shows the caller where they are signed in ("Сессии" of
// openvk's settings): the session that asks is marked.
func (a *api) listSessions(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	sessions, err := a.auth.ListSessions(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := sessionsResponse{Sessions: make([]sessionResponse, 0, len(sessions))}
	for _, s := range sessions {
		resp.Sessions = append(resp.Sessions, sessionResponse{
			ID:         s.ID,
			CreatedAt:  s.CreatedAt,
			LastUsedAt: s.LastUsedAt,
			IP:         s.IP,
			UserAgent:  s.UserAgent,
			Current:    s.Current,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// endSession signs the caller out of one of their sessions.
func (a *api) endSession(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.auth.EndSession(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// endOtherSessions signs the caller out everywhere but here ("Сбросить
// все сессии" on openvk).
func (a *api) endOtherSessions(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ended, err := a.auth.EndOtherSessions(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, endedSessionsResponse{Ended: ended})
}
