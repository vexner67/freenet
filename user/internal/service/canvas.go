package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// CanvasRepository keeps the cells of the canvas and when each person
// last painted.
type CanvasRepository interface {
	// Cells are the cells ever painted.
	Cells(ctx context.Context) ([]user.CanvasCell, error)
	// Cell is the cell x, y; user.ErrNotFound when it was never painted.
	Cell(ctx context.Context, x, y int) (user.CanvasCell, error)
	// PaintedAt is when userID last painted, zero for never.
	PaintedAt(ctx context.Context, userID uuid.UUID) (time.Time, error)
	// Painted writes down that userID painted at, unless they painted
	// after since: user.ErrCanvasCooldown; user.ErrNotFound without them.
	Painted(ctx context.Context, userID uuid.UUID, at, since time.Time) error
	PutCell(ctx context.Context, c user.CanvasCell) error
}

// CanvasView is the canvas, a byte a cell, and when the viewer may paint
// next: zero for now or for a guest.
type CanvasView struct {
	Picture []byte
	NextAt  time.Time
}

// CanvasService is "Холст": a picture all the people paint together.
type CanvasService struct {
	canvas CanvasRepository
	tx     TxManager
}

func NewCanvasService(canvas CanvasRepository, tx TxManager) *CanvasService {
	return &CanvasService{canvas: canvas, tx: tx}
}

// Canvas is the canvas as viewerID sees it at now; uuid.Nil for a guest.
func (s *CanvasService) Canvas(ctx context.Context, viewerID uuid.UUID, now time.Time) (CanvasView, error) {
	cells, err := s.canvas.Cells(ctx)
	if err != nil {
		return CanvasView{}, errs.Wrap("s.canvas.Cells", err)
	}

	view := CanvasView{Picture: user.CanvasPicture(cells)}
	if viewerID == uuid.Nil {
		return view, nil
	}

	last, err := s.canvas.PaintedAt(ctx, viewerID)
	if err != nil {
		return CanvasView{}, errs.Wrap("s.canvas.PaintedAt", err)
	}
	if next := user.CanvasNextAt(last); next.After(now) {
		view.NextAt = next
	}

	return view, nil
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

// Paint paints the cell c and answers when its painter may paint next;
// user.ErrCanvasCooldown within user.CanvasCooldown of their last cell.
func (s *CanvasService) Paint(ctx context.Context, c user.CanvasCell) (time.Time, error) {
	c.At = c.At.Truncate(time.Microsecond)
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.canvas.Painted(ctx, c.UserID, c.At, c.At.Add(-user.CanvasCooldown)); err != nil {
			return errs.Wrap("s.canvas.Painted", err)
		}

		return errs.Wrap("s.canvas.PutCell", s.canvas.PutCell(ctx, c))
	})
	if err != nil {
		return time.Time{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return user.CanvasNextAt(c.At), nil
}
