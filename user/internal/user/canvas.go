package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// The shared canvas ("Холст"): a picture of CanvasSize×CanvasSize cells
// all the people of the site paint together, a cell at a time in one of
// CanvasColors colors; one paints again CanvasCooldown after. A cell
// keeps who painted it last and when.

const (
	CanvasSize     = 100
	CanvasColors   = 16
	CanvasCooldown = time.Minute
)

var (
	ErrCanvasCell     = errors.New("there is no such cell of the canvas")
	ErrCanvasColor    = errors.New("there is no such color of the canvas")
	ErrCanvasCooldown = errors.New("you paint the canvas again a minute after")
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

// CanvasNextAt is when one who last painted at last may paint again, zero
// for one who never painted.
func CanvasNextAt(last time.Time) time.Time {
	if last.IsZero() {
		return time.Time{}
	}

	return last.Add(CanvasCooldown)
}
