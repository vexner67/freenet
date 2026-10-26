package rest

import (
	"image/color"
	"net/http"
	"strings"

	"github.com/boombuler/barcode/qr"
	"github.com/vexner67/freenet/pkg/errs"
)

type twoFactorRequest struct {
	Ticket string `json:"ticket"`
	Code   string `json:"code"`
}

type twoFactorCodeRequest struct {
	Code string `json:"code"`
}

type twoFactorResponse struct {
	Enabled      bool `json:"enabled"`
	RecoveryLeft int  `json:"recoveryLeft"`
}

type twoFactorSetupResponse struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
	// QR is the code of URI, row by row, "1" for a dark module: the page
	// draws it, so the address never leaves the site for a picture.
	QR []string `json:"qr"`
}

type recoveryCodesResponse struct {
	RecoveryCodes []string `json:"recoveryCodes"`
}

// verifyTwoFactor finishes a sign-in that the email code answered with a
// ticket.
func (a *api) verifyTwoFactor(w http.ResponseWriter, r *http.Request) {
	var req twoFactorRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	tokens, err := a.auth.VerifyTwoFactor(r.Context(), req.Ticket, req.Code, a.client(r))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toTokensResponse(tokens))
}

func (a *api) getTwoFactor(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	state, err := a.auth.TwoFactor(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, twoFactorResponse{Enabled: state.Enabled, RecoveryLeft: state.RecoveryLeft})
}

// setUpTwoFactor names the page in the app by its address, or its email
// before it has one.
func (a *api) setUpTwoFactor(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	account := me.Profile.Nickname
	if account == "" {
		account = me.Email
	}

	setup, err := a.auth.SetUpTwoFactor(r.Context(), token, account)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	rows, err := qrRows(setup.URI)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, twoFactorSetupResponse{Secret: setup.Secret, URI: setup.URI, QR: rows})
}

func (a *api) enableTwoFactor(w http.ResponseWriter, r *http.Request) {
	a.withCode(w, r, func(token, code string) (any, error) {
		codes, err := a.auth.EnableTwoFactor(r.Context(), token, code)
		return recoveryCodesResponse{RecoveryCodes: codes}, err
	})
}

func (a *api) disableTwoFactor(w http.ResponseWriter, r *http.Request) {
	a.withCode(w, r, func(token, code string) (any, error) {
		return nil, a.auth.DisableTwoFactor(r.Context(), token, code)
	})
}

func (a *api) renewRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	a.withCode(w, r, func(token, code string) (any, error) {
		codes, err := a.auth.RenewRecoveryCodes(r.Context(), token, code)
		return recoveryCodesResponse{RecoveryCodes: codes}, err
	})
}

// withCode runs a change of the second step confirmed by a code: its
// answer, or 204 for none.
func (a *api) withCode(w http.ResponseWriter, r *http.Request, change func(token, code string) (any, error)) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req twoFactorCodeRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	resp, err := change(token, req.Code)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if resp == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// qrRows is the QR code of s with medium error correction, as rows of
// "0" and "1".
func qrRows(s string) ([]string, error) {
	code, err := qr.Encode(s, qr.M, qr.Auto)
	if err != nil {
		return nil, errs.Wrap("qr.Encode", err)
	}

	bounds := code.Bounds()
	rows := make([]string, 0, bounds.Dy())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		var b strings.Builder
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if code.At(x, y) == color.Black {
				b.WriteByte('1')
			} else {
				b.WriteByte('0')
			}
		}
		rows = append(rows, b.String())
	}

	return rows, nil
}
