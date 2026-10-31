package model

import "time"

// Canvas is the shared canvas: its cells a byte each, row after row, the
// color in the palette.
type Canvas struct {
	Cells []byte
	Size  int
}

// CanvasCell is who painted a cell last (empty when the page is erased),
// the color and when.
type CanvasCell struct {
	UserID    string
	Color     int
	PaintedAt time.Time
}

// CanvasPaint is a cell to paint.
type CanvasPaint struct {
	X     int
	Y     int
	Color int
}
