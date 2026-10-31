package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// CanvasRepository keeps the cells of the canvas.
type CanvasRepository interface {
	// Cells are the cells ever painted.
	Cells(ctx context.Context) ([]user.CanvasCell, error)
	// Cell is the cell x, y; user.ErrNotFound when it was never painted.
	Cell(ctx context.Context, x, y int) (user.CanvasCell, error)
	// PutCell paints a cell; user.ErrNotFound without its painter.
	PutCell(ctx context.Context, c user.CanvasCell) error
}

// CanvasService is "Холст": a picture all the people paint together.
type CanvasService struct {
	canvas CanvasRepository
}

func NewCanvasService(canvas CanvasRepository) *CanvasService {
	return &CanvasService{canvas: canvas}
}

// Canvas is the picture, a byte a cell.
func (s *CanvasService) Canvas(ctx context.Context) ([]byte, error) {
	cells, err := s.canvas.Cells(ctx)
	if err != nil {
		return nil, errs.Wrap("s.canvas.Cells", err)
	}

	return user.CanvasPicture(cells), nil
}

// Cell is who painted the cell x, y last and when; user.ErrCanvasCell off
// the canvas, user.ErrNotFound when it was never painted.
func (s *CanvasService) Cell(ctx context.Context, x, y int) (user.CanvasCell, error) {
	if _, err := user.NewCanvasCell(x, y, 0, uuid.Nil, time.Time{}); err != nil {
		return user.CanvasCell{}, errs.Wrap("user.NewCanvasCell", err)
	}

	c, err := s.canvas.Cell(ctx, x, y)

	return c, errs.Wrap("s.canvas.Cell", err)
}

// Paint paints the cell c.
func (s *CanvasService) Paint(ctx context.Context, c user.CanvasCell) error {
	c.At = c.At.Truncate(time.Microsecond)

	return errs.Wrap("s.canvas.PutCell", s.canvas.PutCell(ctx, c))
}
