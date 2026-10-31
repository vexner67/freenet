package rest

import (
	"context"
	"net/http"
	"strconv"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// Canvas is "Холст", the canvas everybody paints.
type Canvas interface {
	Canvas(ctx context.Context, accessToken string) (model.Canvas, error)
	Paint(ctx context.Context, accessToken string, p model.CanvasPaint) error
	Cell(ctx context.Context, accessToken string, x, y int) (model.CanvasCell, error)
}

// canvasJSON is the canvas: cells a byte each, row after row, in base64.
type canvasJSON struct {
	Cells []byte `json:"cells"`
	Size  int    `json:"size"`
}

// getCanvas is the canvas, public.
func (a *api) getCanvas(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.canvas.Canvas(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, canvasJSON{Cells: c.Cells, Size: c.Size})
}

type canvasPaintRequest struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Color int `json:"color"`
}

// paintCanvas paints a cell.
func (a *api) paintCanvas(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req canvasPaintRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.canvas.Paint(r.Context(), token, model.CanvasPaint(req)); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getCanvasCell is who painted a cell last, public; user is null when
// their page is gone.
func (a *api) getCanvasCell(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	x, errX := strconv.Atoi(r.PathValue("x"))
	y, errY := strconv.Atoi(r.PathValue("y"))
	if errX != nil || errY != nil {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "there is no such cell of the canvas"))
		return
	}

	c, err := a.canvas.Cell(r.Context(), token, x, y)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	people, err := a.people(r, []string{c.UserID})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		User      *authorJSON `json:"user"`
		Color     int         `json:"color"`
		PaintedAt string      `json:"paintedAt"`
	}{petOwner(c.UserID, people), c.Color, timeString(c.PaintedAt)})
}
