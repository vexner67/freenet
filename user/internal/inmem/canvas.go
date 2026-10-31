package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// CanvasRepository keeps the canvas in memory; a deleted user leaves
// their cells without a painter, as the foreign key does.
type CanvasRepository struct {
	users *UserRepository

	mu    sync.Mutex
	cells map[[2]int]user.CanvasCell
}

func NewCanvasRepository(users *UserRepository) *CanvasRepository {
	r := &CanvasRepository{users: users, cells: map[[2]int]user.CanvasCell{}}
	users.onDelete(r.forget)

	return r
}

func (r *CanvasRepository) forget(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for k, c := range r.cells {
		if c.UserID == id {
			c.UserID = uuid.Nil
			r.cells[k] = c
		}
	}
}

func (r *CanvasRepository) Cells(ctx context.Context) ([]user.CanvasCell, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	cells := make([]user.CanvasCell, 0, len(r.cells))
	for _, c := range r.cells {
		cells = append(cells, c)
	}

	return cells, nil
}

func (r *CanvasRepository) Cell(ctx context.Context, x, y int) (user.CanvasCell, error) {
	if err := ctx.Err(); err != nil {
		return user.CanvasCell{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.cells[[2]int{x, y}]
	if !ok {
		return user.CanvasCell{}, user.ErrNotFound
	}

	return c, nil
}

func (r *CanvasRepository) PutCell(ctx context.Context, c user.CanvasCell) error {
	if _, err := r.users.GetByID(ctx, c.UserID); err != nil {
		return errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.cells[[2]int{c.X, c.Y}] = c

	return nil
}
