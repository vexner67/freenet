package rest_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakeGames keeps one save; the records are Bob's (u2) 70, Ann's (u1) 10
// and u3's, whose page is deleted.
type fakeGames struct {
	token, game, data string
	saved             bool
	score             int64
	players           []string
	err               error
}

func (f *fakeGames) Save(_ context.Context, token, game, data string) error {
	f.token, f.game, f.data, f.saved = token, game, data, true
	return f.err
}

func (f *fakeGames) Load(_ context.Context, token, game string) (string, bool, error) {
	f.token, f.game = token, game
	return f.data, f.saved, f.err
}

func (f *fakeGames) Score(_ context.Context, token, game string, score int64) (int64, error) {
	f.token, f.game, f.score = token, game, score
	return max(score, 70), f.err
}

func (f *fakeGames) Records(_ context.Context, token, game string, players []string, _ int) ([]model.GameRecord, error) {
	f.token, f.game, f.players = token, game, players
	return []model.GameRecord{{UserID: "u2", Score: 70}, {UserID: "u3", Score: 20}, {UserID: "u1", Score: 10}}, f.err
}

func TestGames(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/games/g1/save", "", "Authorization", "Bearer tok")
	if got := decode[struct{ Found bool }](t, w); w.Code != http.StatusOK || got.Found {
		t.Fatalf("load before a save: %d %+v", w.Code, got)
	}

	if w = h.do("PUT", "/api/v1/games/g1/save", `{"data":"{\"level\":3}"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	if h.games.token != "tok" || h.games.game != "g1" || h.games.data != `{"level":3}` {
		t.Fatalf("saved = %+v", h.games)
	}

	w = h.do("GET", "/api/v1/games/g1/save", "", "Authorization", "Bearer tok")
	if got := decode[struct {
		Found bool
		Data  string
	}](t, w); !got.Found || got.Data != `{"level":3}` {
		t.Fatalf("load: %+v", got)
	}

	w = h.do("POST", "/api/v1/games/g1/scores", `{"score":40}`, "Authorization", "Bearer tok")
	if got := decode[struct{ Best int64 }](t, w); w.Code != http.StatusOK || got.Best != 70 || h.games.score != 40 {
		t.Fatalf("score: %d %+v", w.Code, got)
	}

	// The caller and their friends, the deleted page left out.
	w = h.do("GET", "/api/v1/games/g1/records", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Records []struct {
			User  struct{ ID, Name string }
			Score int64
		}
	}](t, w)
	if w.Code != http.StatusOK || len(got.Records) != 2 || got.Records[0].User.ID != "u2" || got.Records[0].Score != 70 || got.Records[1].User.ID != "u1" {
		t.Fatalf("records: %d %+v", w.Code, got)
	}
	if !slices.Equal(h.games.players, []string{"u1", "u2"}) {
		t.Fatalf("players = %v", h.games.players)
	}

	// A guest neither plays nor sees records.
	for _, req := range [][2]string{{"GET", "/api/v1/games/g1/save"}, {"PUT", "/api/v1/games/g1/save"}, {"POST", "/api/v1/games/g1/scores"}, {"GET", "/api/v1/games/g1/records"}} {
		if w = h.do(req[0], req[1], `{}`); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s as a guest: %d", req[0], req[1], w.Code)
		}
	}

	h = newHarness(rest.Config{})
	h.games.err = failure.New(failure.InvalidArgument, "a game save is at most 64 KB")
	if w = h.do("PUT", "/api/v1/games/g1/save", `{"data":"x"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("too large: %d", w.Code)
	}
}
