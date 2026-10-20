package rest

import (
	"context"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// Decor is the New Year decoration of the site, which the user service
// keeps: everyone asks whether to wear it, the administrators of the
// site turn it on and off.
type Decor interface {
	Get(ctx context.Context) (model.Decor, error)
	// Set is "auto", "on" or "off".
	Set(ctx context.Context, accessToken, mode string) error
}

type decorJSON struct {
	Mode    string `json:"mode"`
	Festive bool   `json:"festive"`
}

// decor is public: every page, a guest's too, asks whether to wear the
// New Year decoration.
func (a *api) decor(w http.ResponseWriter, r *http.Request) {
	d, err := a.decors.Get(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, decorJSON{Mode: d.Mode, Festive: d.Festive})
}

// setDecor is "Оформление" of the admin panel.
func (a *api) setDecor(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Mode string `json:"mode"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.decors.Set(r.Context(), token, req.Mode); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
