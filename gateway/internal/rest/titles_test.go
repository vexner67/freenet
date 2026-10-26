package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeTitles struct {
	token    string
	nickname string
	name     string
	icon     string
	revoked  string
	err      error
}

func (f *fakeTitles) title() model.Title {
	return model.Title{ID: "t1", Name: "Король шахты", Icon: "crown", GrantedAt: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)}
}

func (f *fakeTitles) UserTitles(_ context.Context, token, nickname string) ([]model.Title, error) {
	f.token, f.nickname = token, nickname
	return []model.Title{f.title()}, f.err
}

func (f *fakeTitles) Grant(_ context.Context, token, nickname, name, icon string) (model.Title, error) {
	f.token, f.nickname, f.name, f.icon = token, nickname, name, icon
	return f.title(), f.err
}

func (f *fakeTitles) Revoke(_ context.Context, token, id string) error {
	f.token, f.revoked = token, id
	return f.err
}

func TestTitles(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	w := h.do("GET", "/api/v1/users/ann/titles", "")
	got := decode[map[string]any](t, w)["titles"].([]any)
	if w.Code != http.StatusOK || h.titles.nickname != "ann" || len(got) != 1 ||
		got[0].(map[string]any)["icon"] != "crown" || got[0].(map[string]any)["grantedAt"] != "2026-10-26T12:00:00Z" {
		t.Fatalf("titles of a page: %d %v", w.Code, got)
	}

	if w = h.do("POST", "/api/v1/users/ann/titles", `{"name":"Король шахты","icon":"crown"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("grant by a guest: %d", w.Code)
	}
	w = h.do("POST", "/api/v1/users/ann/titles", `{"name":"Король шахты","icon":"crown"}`, auth...)
	if w.Code != http.StatusCreated || h.titles.token != "tok" || h.titles.name != "Король шахты" || h.titles.icon != "crown" ||
		decode[map[string]any](t, w)["id"] != "t1" {
		t.Fatalf("grant: %d %s", w.Code, w.Body)
	}

	if w = h.do("DELETE", "/api/v1/titles/t1", "", auth...); w.Code != http.StatusNoContent || h.titles.revoked != "t1" {
		t.Fatalf("revoke: %d", w.Code)
	}

	h.titles.err = failure.New(failure.PermissionDenied, "permission denied")
	if w = h.do("DELETE", "/api/v1/titles/t1", "", auth...); w.Code != http.StatusForbidden {
		t.Fatalf("revoke by another: %d", w.Code)
	}
}
