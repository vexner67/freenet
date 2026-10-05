package rest

import (
	"net/http"
	"strings"

	"github.com/vexner67/freenet/gateway/internal/failure"
)

func (a *api) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// noRoute answers 405 when the path exists for other methods, else 404.
func (a *api) noRoute(mux *http.ServeMux, w http.ResponseWriter, r *http.Request) {
	var allowed []string

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		probe := r.Clone(r.Context())
		probe.Method = method

		if _, pattern := mux.Handler(probe); pattern != "" && pattern != "/" {
			allowed = append(allowed, method)
		}
	}

	if len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		a.writeError(w, r, failure.New(failure.MethodNotAllowed, "method not allowed"))
		return
	}

	a.writeError(w, r, failure.New(failure.NotFound, "no such endpoint"))
}

func (a *api) requestCode(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err := a.auth.RequestCode(r.Context(), req.Email, clientIP(r, a.trustForwardedFor)); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) verifyCode(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	tokens, err := a.auth.VerifyCode(r.Context(), req.Email, req.Code)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toTokensResponse(tokens))
}

func (a *api) refreshToken(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	tokens, err := a.auth.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toTokensResponse(tokens))
}

func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err := a.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) getMe(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	u, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toUserResponse(u))
}

func (a *api) updateProfile(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req profileJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	profile, err := req.toModel()
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	u, err := a.users.UpdateProfile(r.Context(), token, profile)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toUserResponse(u))
}

func (a *api) updatePrivacy(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req privacyJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	u, err := a.users.UpdatePrivacy(r.Context(), token, req.toModel())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toUserResponse(u))
}

// getProfile works without a token; with one it also tells whether the
// caller follows the user.
func (a *api) getProfile(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	profile, err := a.users.GetProfile(r.Context(), r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	stats, err := a.users.FollowStats(r.Context(), token, profile.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, profileWithStatsResponse{
		publicProfileResponse: toPublicProfileResponse(profile),
		followStatsJSON:       toFollowStatsJSON(stats),
	})
}
