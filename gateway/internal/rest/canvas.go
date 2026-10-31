package rest

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

const (
	canvasLivePath = "/api/v1/canvas/live"

	// canvasLag is how many cells a page may lag behind before it is cut
	// off, to connect again and take the whole canvas.
	canvasLag = 256
	// maxCanvasStreams bounds the streams anyone may open without a token.
	maxCanvasStreams = 10_000
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

	a.paints.Publish(r.Context(), struct{}{}, req)
	w.WriteHeader(http.StatusNoContent)
}

// canvasLive streams each cell as it is painted (WebSocket), public: the
// canvas is public too. 1013 asks to connect again a bit later.
func (a *api) canvasLive(w http.ResponseWriter, r *http.Request) {
	// As in events: the server's deadlines would cut a long connection.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	// Before the answer: a page reads the canvas once connected and must
	// not miss a cell painted in between.
	painted, unsubscribe := a.paints.Subscribe(struct{}{})
	defer unsubscribe()

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: a.eventOrigins})
	if err != nil {
		return // Accept has answered
	}
	defer conn.CloseNow() //nolint:errcheck // the connection may be gone already

	a.streams.Add(1)
	defer a.streams.Done()

	if a.canvasStreams.Add(1) > maxCanvasStreams {
		a.canvasStreams.Add(-1)
		a.closeEvents(r.Context(), conn, failure.New(failure.Unavailable, "too many canvases open"))

		return
	}
	defer a.canvasStreams.Add(-1)

	a.eventStreams.Inc()
	defer a.eventStreams.Dec()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	stop := context.AfterFunc(a.stopping, cancel)
	defer stop()

	a.closeEvents(r.Context(), conn, streamCanvas(ctx, conn, painted))
}

// streamCanvas writes the painted cells until the page goes, as a page
// sends nothing; a page that fell behind ends the stream.
func streamCanvas(ctx context.Context, conn *websocket.Conn, painted <-chan canvasPaintRequest) error {
	gone := conn.CloseRead(context.Background())

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(gone, cancel)()

	pings := make(chan error, 1)
	go func() { pings <- ping(ctx, conn) }()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-pings:
			return err
		case cell, ok := <-painted:
			if !ok {
				return failure.New(failure.Unavailable, "the page fell behind the canvas")
			}

			writeCtx, cancelWrite := context.WithTimeout(ctx, writeWait)
			err := wsjson.Write(writeCtx, conn, cell)
			cancelWrite()
			if err != nil {
				return err
			}
		}
	}
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
