package rest_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestUpdateStatus(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile.Nickname = "ann"

	w := h.do("PUT", "/api/v1/me/status", `{"status":"Привет"}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Status       string   `json:"status"`
		Completeness int      `json:"completeness"`
		Unfilled     []string `json:"unfilled"`
	}](t, w)
	if w.Code != http.StatusOK || got.Status != "Привет" || got.Completeness != 85 || !slices.Equal(got.Unfilled, []string{"telegram"}) {
		t.Fatalf("%d %+v", w.Code, got)
	}

	if w = h.do("PUT", "/api/v1/me/status", `{"status":"x"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
}

// "Адрес страницы" is saved alone, without the rest of the profile.
func TestUpdateNickname(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile.Nickname = "ann"
	h.users.profile.Status = "Привет"

	w := h.do("PUT", "/api/v1/me/nickname", `{"nickname":"anna"}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Nickname string `json:"nickname"`
		Status   string `json:"status"`
	}](t, w)
	if w.Code != http.StatusOK || got.Nickname != "anna" || got.Status != "Привет" || h.users.token != "tok" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	h.users.err = failure.New(failure.Conflict, "nickname is already taken")
	if w = h.do("PUT", "/api/v1/me/nickname", `{"nickname":"bob"}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("taken: %d", w.Code)
	}

	if w = h.do("PUT", "/api/v1/me/nickname", `{"nickname":"x"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
}

func TestWallTabs(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	for _, tab := range []string{"owner", "others"} {
		if w := h.do("GET", "/api/v1/users/ann/posts?type="+tab, ""); w.Code != http.StatusOK || h.posts.wallFilter != tab {
			t.Fatalf("%s: %d, asked for %q", tab, w.Code, h.posts.wallFilter)
		}
	}
	if w := h.do("GET", "/api/v1/clubs/cats/posts?type=others", ""); w.Code != http.StatusOK || h.posts.wallFilter != "others" {
		t.Fatalf("the club's: %d, asked for %q", w.Code, h.posts.wallFilter)
	}
	// "Архив записей" and "Поиск".
	if w := h.do("GET", "/api/v1/users/ann/posts?type=archive&q=%D0%BC%D0%BE%D1%80%D0%B5", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.posts.wallFilter != "archive" || h.posts.wallQuery != "море" {
		t.Fatalf("the archive searched: %d, asked for %q and %q", w.Code, h.posts.wallFilter, h.posts.wallQuery)
	}
	if w := h.do("GET", "/api/v1/clubs/cats/posts?q=x", ""); w.Code != http.StatusOK || h.posts.wallQuery != "x" {
		t.Fatalf("the club's searched: %d, asked for %q", w.Code, h.posts.wallQuery)
	}
	// Only the archive tells the years of the wall.
	h.posts.yearsOf = ""
	if w := h.do("GET", "/api/v1/users/ann/posts?type=owner", ""); h.posts.yearsOf != "" || strings.Contains(w.Body.String(), "years") {
		t.Fatalf("the years of a wall asked for outside its archive: %q", h.posts.yearsOf)
	}
	if w := h.do("GET", "/api/v1/users/ann/posts", ""); w.Code != http.StatusOK || h.posts.wallFilter != "" {
		t.Fatalf("all: %d, asked for %q", w.Code, h.posts.wallFilter)
	}
}

func TestArchiveByYears(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	// The first page of the archive tells the years of the wall.
	w := h.do("GET", "/api/v1/users/ann/posts?type=archive&year=2024", "", auth...)
	if w.Code != http.StatusOK || h.posts.wallYear != 2024 || h.posts.yearsOf != "u1" || !strings.Contains(w.Body.String(), `"years":[2026,2024]`) {
		t.Fatalf("the archive of 2024: %d %s, asked for %d of %q", w.Code, w.Body, h.posts.wallYear, h.posts.yearsOf)
	}
	h.posts.yearsOf = ""
	if w = h.do("GET", "/api/v1/users/ann/posts?type=archive&cursor=p1", "", auth...); w.Code != http.StatusOK || h.posts.yearsOf != "" || h.posts.wallYear != 0 {
		t.Fatalf("the next page: %d, the years of %q, year %d", w.Code, h.posts.yearsOf, h.posts.wallYear)
	}
	if w = h.do("GET", "/api/v1/clubs/cats/posts?type=archive&year=2025", "", auth...); w.Code != http.StatusOK || h.posts.yearsOf != "k1" || h.posts.wallYear != 2025 {
		t.Fatalf("the archive of a club: %d, the years of %q, year %d", w.Code, h.posts.yearsOf, h.posts.wallYear)
	}
	for _, year := range []string{"x", "0", "-5", "2025.5"} {
		if w = h.do("GET", "/api/v1/users/ann/posts?type=archive&year="+year, "", auth...); w.Code != http.StatusBadRequest {
			t.Errorf("year %q: %d", year, w.Code)
		}
	}

	// "Архивировать все записи за 2025" and the rest.
	for _, c := range []struct{ method, path, want string }{
		{"PUT", "/api/v1/users/ann/posts/archive?year=2025", "u1 2025 true"},
		{"DELETE", "/api/v1/users/ann/posts/archive?year=2025", "u1 2025 false"},
		{"PUT", "/api/v1/users/ann/posts/archive", "u1 0 true"},
		{"PUT", "/api/v1/clubs/cats/posts/archive?year=2024", "k1 2024 true"},
		{"DELETE", "/api/v1/clubs/cats/posts/archive", "k1 0 false"},
	} {
		w = h.do(c.method, c.path, "", auth...)
		if w.Code != http.StatusOK || h.posts.archivedWall != c.want || strings.TrimSpace(w.Body.String()) != `{"changed":3}` {
			t.Errorf("%s %s: %d %s, archived %q", c.method, c.path, w.Code, w.Body, h.posts.archivedWall)
		}
	}

	if w = h.do("PUT", "/api/v1/users/ann/posts/archive", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
	if w = h.do("PUT", "/api/v1/users/ann/posts/archive?year=x", "", auth...); w.Code != http.StatusBadRequest {
		t.Fatalf("a bad year: %d", w.Code)
	}
	if w = h.do("PUT", "/api/v1/users/nobody/posts/archive", "", auth...); w.Code != http.StatusNotFound {
		t.Fatalf("the wall of nobody: %d", w.Code)
	}

	h.posts.err = failure.New(failure.PermissionDenied, "permission denied")
	if w = h.do("PUT", "/api/v1/users/ann/posts/archive", "", auth...); w.Code != http.StatusForbidden {
		t.Fatalf("a stranger archives the wall: %d", w.Code)
	}
}
