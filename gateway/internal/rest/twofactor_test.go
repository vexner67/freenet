package rest_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// The second step of the fake takes "654321" and a ticket "ticket".
func (f *fakeAuth) twoFactorCode(code string) error {
	if code != "654321" {
		return failure.New(failure.InvalidArgument, "invalid two-factor code")
	}
	return nil
}

func (f *fakeAuth) VerifyTwoFactor(_ context.Context, ticket, code string, client model.Client) (model.Tokens, error) {
	f.client = client
	if ticket != "ticket" {
		return model.Tokens{}, failure.New(failure.Unauthenticated, "sign-in expired, request a new code")
	}
	if err := f.twoFactorCode(code); err != nil {
		return model.Tokens{}, err
	}
	return model.Tokens{AccessToken: "access-bob", RefreshToken: "refresh"}, nil
}

func (f *fakeAuth) TwoFactor(context.Context, string) (model.TwoFactor, error) {
	return model.TwoFactor{Enabled: f.twoFactorOn, RecoveryLeft: 9}, f.err
}

func (f *fakeAuth) SetUpTwoFactor(_ context.Context, _, account string) (model.TwoFactorSetup, error) {
	f.account = account
	return model.TwoFactorSetup{Secret: "GEZD GNBV", URI: "otpauth://totp/Freenet:" + account + "?secret=GEZDGNBV&issuer=Freenet"}, f.err
}

func (f *fakeAuth) EnableTwoFactor(_ context.Context, _, code string) ([]string, error) {
	if err := f.twoFactorCode(code); err != nil {
		return nil, err
	}
	f.twoFactorOn = true
	return []string{"abcde-fghij", "kmnpq-rstuv"}, nil
}

func (f *fakeAuth) DisableTwoFactor(_ context.Context, _, code string) error {
	if err := f.twoFactorCode(code); err != nil {
		return err
	}
	f.twoFactorOn = false
	return nil
}

func (f *fakeAuth) RenewRecoveryCodes(_ context.Context, _, code string) ([]string, error) {
	if err := f.twoFactorCode(code); err != nil {
		return nil, err
	}
	return []string{"wxyz2-34567"}, nil
}

func TestTwoFactorSignIn(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	// The email code of a page with the second step on brings no tokens.
	w := h.do("POST", "/api/v1/auth/verify", `{"email":"bob@example.com","code":"123456"}`)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "accessToken") {
		t.Fatalf("verify = %d %s", w.Code, w.Body)
	}
	if got := decode[map[string]string](t, w); got["twoFactorTicket"] != "ticket" {
		t.Fatalf("verify = %v", got)
	}

	if w = h.do("POST", "/api/v1/auth/two-factor", `{"ticket":"ticket","code":"000000"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a wrong code = %d %s", w.Code, w.Body)
	}
	if w = h.do("POST", "/api/v1/auth/two-factor", `{"ticket":"other","code":"654321"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("a wrong ticket = %d %s", w.Code, w.Body)
	}

	w = h.do("POST", "/api/v1/auth/two-factor", `{"ticket":"ticket","code":"654321"}`, "User-Agent", "Firefox")
	tokens := decode[map[string]string](t, w)
	if w.Code != http.StatusOK || tokens["accessToken"] != "access-bob" || tokens["refreshToken"] != "refresh" {
		t.Fatalf("two-factor = %d %v", w.Code, tokens)
	}
	if h.auth.client.IP != "203.0.113.9" || h.auth.client.UserAgent != "Firefox" {
		t.Fatalf("client = %+v", h.auth.client)
	}
}

func TestTwoFactorSettings(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile.Nickname = "ann"
	auth := []string{"Authorization", "Bearer tok"}

	if w := h.do("GET", "/api/v1/me/two-factor", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token = %d", w.Code)
	}

	w := h.do("POST", "/api/v1/me/two-factor/setup", "", auth...)
	setup := decode[struct {
		Secret string   `json:"secret"`
		URI    string   `json:"uri"`
		QR     []string `json:"qr"`
	}](t, w)
	if w.Code != http.StatusOK || setup.Secret != "GEZD GNBV" || h.auth.account != "ann" || !strings.Contains(setup.URI, "Freenet:ann") {
		t.Fatalf("setup = %d %+v", w.Code, setup)
	}
	// A square of 0 and 1 with the finder pattern in the corner.
	if len(setup.QR) < 21 || len(setup.QR[0]) != len(setup.QR) || !strings.HasPrefix(setup.QR[0], "1111111") || strings.Trim(strings.Join(setup.QR, ""), "01") != "" {
		t.Fatalf("qr = %v", setup.QR)
	}

	if w = h.do("POST", "/api/v1/me/two-factor/enable", `{"code":"1"}`, auth...); w.Code != http.StatusBadRequest {
		t.Fatalf("enable with a wrong code = %d", w.Code)
	}

	w = h.do("POST", "/api/v1/me/two-factor/enable", `{"code":"654321"}`, auth...)
	if got := decode[map[string][]string](t, w); w.Code != http.StatusOK || len(got["recoveryCodes"]) != 2 {
		t.Fatalf("enable = %d %v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/me/two-factor", "", auth...)
	if got := decode[map[string]any](t, w); got["enabled"] != true || got["recoveryLeft"] != float64(9) {
		t.Fatalf("state = %v", got)
	}

	w = h.do("POST", "/api/v1/me/two-factor/recovery", `{"code":"654321"}`, auth...)
	if got := decode[map[string][]string](t, w); w.Code != http.StatusOK || got["recoveryCodes"][0] != "wxyz2-34567" {
		t.Fatalf("recovery = %d %v", w.Code, got)
	}

	if w = h.do("POST", "/api/v1/me/two-factor/disable", `{"code":"654321"}`, auth...); w.Code != http.StatusNoContent || h.auth.twoFactorOn {
		t.Fatalf("disable = %d", w.Code)
	}
}
