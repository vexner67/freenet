package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// The shared canvas ("Холст"): a picture of CanvasSize×CanvasSize cells
// all the people of the site paint together, a cell at a time in one of
// CanvasColors colors. A cell keeps who painted it last and when.

const (
	CanvasSize   = 100
	CanvasColors = 64
)

var (
	ErrCanvasCell  = errors.New("there is no such cell of the canvas")
	ErrCanvasColor = errors.New("there is no such color of the canvas")
)

// CanvasCell is a cell of the canvas painted Color by UserID at At.
type CanvasCell struct {
	X, Y   int
	Color  int
	UserID uuid.UUID
	At     time.Time
}

// NewCanvasCell is the cell x, y painted color by userID at at:
// ErrCanvasCell off the canvas, ErrCanvasColor out of the palette.
func NewCanvasCell(x, y, color int, userID uuid.UUID, at time.Time) (CanvasCell, error) {
	if x < 0 || y < 0 || x >= CanvasSize || y >= CanvasSize {
		return CanvasCell{}, ErrCanvasCell
	}
	if color < 0 || color >= CanvasColors {
		return CanvasCell{}, ErrCanvasColor
	}

	return CanvasCell{X: x, Y: y, Color: color, UserID: userID, At: at}, nil
}

// CanvasPicture is the canvas of the cells painted, a byte a cell, row
// after row; a cell never painted is of color 0.
func CanvasPicture(cells []CanvasCell) []byte {
	picture := make([]byte, CanvasSize*CanvasSize)
	for _, c := range cells {
		picture[c.Y*CanvasSize+c.X] = byte(c.Color) //nolint:gosec // NewCanvasCell keeps it below CanvasColors
	}

	return picture
}
