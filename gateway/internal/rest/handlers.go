package rest

import (
	"net/http"
	"strings"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
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

	a.writeUser(w, r, u)
}

// writeUser answers with the caller's own account and whom its
// relationship names.
func (a *api) writeUser(w http.ResponseWriter, r *http.Request, u model.User) {
	partner, err := a.partner(r, u.Profile.PartnerID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toUserResponse(u, partner))
}

// partner is whom a relationship names, nil for nobody and when that
// page is deleted or erased.
func (a *api) partner(r *http.Request, id string) (*authorJSON, error) {
	if id == "" {
		return nil, nil
	}

	people, err := a.people(r, []string{id})
	if err != nil {
		return nil, err
	}

	p, ok := people[id]
	if !ok || p.Deactivated {
		return nil, nil
	}

	author := toAuthorJSON(p)

	return &author, nil
}

func (a *api) updateProfile(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req profileRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	profile, err := req.toModel()
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	u, err := a.users.UpdateProfile(r.Context(), token, profile, req.Partner)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeUser(w, r, u)
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

	a.writeUser(w, r, u)
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

	// The blacklist is the viewer's: anonymous viewers have none.
	var rel model.Relation
	if token != "" {
		if rel, err = a.users.Relation(r.Context(), token, profile.ID); err != nil {
			a.writeError(w, r, err)
			return
		}
	}

	partner, err := a.partner(r, profile.Profile.PartnerID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, profileWithStatsResponse{
		publicProfileResponse: toPublicProfileResponse(profile),
		aboutJSON:             toAboutJSON(profile.About, partner),
		followStatsJSON:       toFollowStatsJSON(stats),
		relationJSON:          relationJSON{BlockedByMe: rel.BlockedByMe, BlockedMe: rel.BlockedMe},
	})
}

// updateContacts saves "Контакты" of "Редактировать страницу".
func (a *api) updateContacts(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req contactsJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	u, err := a.users.UpdateContacts(r.Context(), token, model.Contacts(req))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeUser(w, r, u)
}

// updateInterests saves "Интересы" of "Редактировать страницу".
func (a *api) updateInterests(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req interestsJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	u, err := a.users.UpdateInterests(r.Context(), token, model.Interests(req))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeUser(w, r, u)
}

// updateExtraFields saves "Дополнительно" of "Редактировать страницу".
func (a *api) updateExtraFields(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req extraFieldsRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	u, err := a.users.UpdateExtraFields(r.Context(), token, req.toModel())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeUser(w, r, u)
}
