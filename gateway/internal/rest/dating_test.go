package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeCanvas struct {
	token string
	paint model.CanvasPaint
}

func (f *fakeCanvas) Canvas(_ context.Context, token string) (model.Canvas, error) {
	f.token = token

	return model.Canvas{Cells: []byte{0, 1, 2, 3}, Size: 2}, nil
}

func (f *fakeCanvas) Paint(_ context.Context, token string, p model.CanvasPaint) error {
	f.token, f.paint = token, p

	return nil
}

// Cell was painted by u3, whose page is deleted, at (1, 1) and by u1
// elsewhere.
func (f *fakeCanvas) Cell(_ context.Context, token string, x, y int) (model.CanvasCell, error) {
	f.token = token
	painter := "u1"
	if x == 1 && y == 1 {
		painter = "u3"
	}

	return model.CanvasCell{UserID: painter, Color: 5, PaintedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}, nil
}

type fakeDating struct {
	saved  model.DatingCardInput
	answer string
	like   bool
}

// Dating shows u2 and u3, a deleted page, and matched u1.
func (f *fakeDating) Dating(context.Context, string) (model.Dating, error) {
	return model.Dating{
		Card:    &model.DatingCard{UserID: "me", Goal: "love", On: true},
		Deck:    []model.DatingCard{{UserID: "u2", Goal: "chat", About: "привет", On: true}, {UserID: "u3", Goal: "love", On: true}},
		Matches: []model.DatingMatch{{UserID: "u1", At: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}},
	}, nil
}

func (f *fakeDating) SaveCard(_ context.Context, _ string, in model.DatingCardInput) (model.DatingCard, error) {
	f.saved = in

	return model.DatingCard{UserID: "me", Goal: in.Goal, Seek: in.Seek, About: in.About, On: in.On}, nil
}

func (f *fakeDating) Answer(_ context.Context, _, userID string, like bool) (bool, error) {
	f.answer, f.like = userID, like

	return like, nil
}

func TestCanvas(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	w := h.do("GET", "/api/v1/canvas", "")
	got := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || got["cells"] != "AAECAw==" || got["size"] != 2.0 || h.canvas.token != "" {
		t.Fatalf("the canvas of a guest: %d %v", w.Code, got)
	}
	if w = h.do("POST", "/api/v1/canvas/cells", `{"x":1,"y":2,"color":3}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("a guest paints: %d", w.Code)
	}
	w = h.do("POST", "/api/v1/canvas/cells", `{"x":1,"y":2,"color":3}`, auth...)
	if w.Code != http.StatusNoContent || h.canvas.paint != (model.CanvasPaint{X: 1, Y: 2, Color: 3}) {
		t.Fatalf("paint: %d", w.Code)
	}

	if w = h.do("GET", "/api/v1/canvas/cells/a/1", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("a cell of no number: %d", w.Code)
	}
	w = h.do("GET", "/api/v1/canvas/cells/0/1", "")
	if got = decode[map[string]any](t, w); w.Code != http.StatusOK || got["user"].(map[string]any)["nickname"] != "ann" || got["color"] != 5.0 {
		t.Fatalf("a cell: %d %v", w.Code, got)
	}
	w = h.do("GET", "/api/v1/canvas/cells/1/1", "")
	if got = decode[map[string]any](t, w); w.Code != http.StatusOK || got["user"] != nil {
		t.Fatalf("a cell of a deleted page: %d %v", w.Code, got)
	}
}

func TestDating(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	if w := h.do("GET", "/api/v1/me/dating", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("dating of a guest: %d", w.Code)
	}

	// The card of the deleted page u3 is left out.
	w := h.do("GET", "/api/v1/me/dating", "", auth...)
	got := decode[map[string]any](t, w)
	deck, matches := got["deck"].([]any), got["matches"].([]any)
	if w.Code != http.StatusOK || got["card"].(map[string]any)["goal"] != "love" || len(deck) != 1 ||
		deck[0].(map[string]any)["user"].(map[string]any)["nickname"] != "bob" || deck[0].(map[string]any)["about"] != "привет" ||
		len(matches) != 1 || matches[0].(map[string]any)["user"].(map[string]any)["nickname"] != "ann" {
		t.Fatalf("dating: %d %v", w.Code, got)
	}

	w = h.do("PUT", "/api/v1/me/dating", `{"goal":"chat","seek":"female","about":"я","on":true}`, auth...)
	if got = decode[map[string]any](t, w); w.Code != http.StatusOK || h.dating.saved != (model.DatingCardInput{Goal: "chat", Seek: "female", About: "я", On: true}) || got["seek"] != "female" {
		t.Fatalf("save: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/me/dating/answers", `{"userId":"u2","like":true}`, auth...)
	if got = decode[map[string]any](t, w); w.Code != http.StatusOK || h.dating.answer != "u2" || !h.dating.like || got["matched"] != true {
		t.Fatalf("answer: %d %v", w.Code, got)
	}
}
