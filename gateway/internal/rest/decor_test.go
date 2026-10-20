package rest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeDecor struct {
	token, mode string
	err         error
}

func (f *fakeDecor) Get(context.Context) (model.Decor, error) {
	return model.Decor{Mode: f.mode, Festive: f.mode == "on"}, nil
}

func (f *fakeDecor) Set(_ context.Context, token, mode string) error {
	if f.err != nil {
		return f.err
	}
	f.token, f.mode = token, mode

	return nil
}

func TestDecor(t *testing.T) {
	t.Parallel()

	// Every page asks, a guest's too.
	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/decor", "")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["mode"] != "auto" || got["festive"] != false {
		t.Fatalf("%d %v", w.Code, got)
	}

	if w = h.do("PUT", "/api/v1/decor", `{"mode":"on"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.decor.token != "tok" {
		t.Fatalf("set: %d, %+v", w.Code, h.decor)
	}
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/decor", "")); got["mode"] != "on" || got["festive"] != true {
		t.Fatalf("after: %v", got)
	}

	if w = newHarness(rest.Config{}).do("PUT", "/api/v1/decor", `{"mode":"on"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}

	h = newHarness(rest.Config{})
	h.decor.err = failure.New(failure.PermissionDenied, "permission denied")
	if w = h.do("PUT", "/api/v1/decor", `{"mode":"off"}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("not an administrator: %d", w.Code)
	}
}
