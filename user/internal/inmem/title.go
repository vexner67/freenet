package inmem

import (
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// TitleRepository keeps the titles of pages in memory; a deleted user
// takes their titles, as the foreign key does.
type TitleRepository struct {
	users *UserRepository

	mu     sync.Mutex
	titles []*user.Title
}

func NewTitleRepository(users *UserRepository) *TitleRepository {
	r := &TitleRepository{users: users}
	users.onDelete(r.forget)

	return r
}

func (r *TitleRepository) forget(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.titles = slices.DeleteFunc(r.titles, func(t *user.Title) bool { return t.UserID() == id })
}

func (r *TitleRepository) AddTitle(ctx context.Context, t *user.Title) error {
	if _, err := r.users.GetByID(ctx, t.UserID()); err != nil {
		return errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.titles = append(r.titles, t)

	return nil
}

func (r *TitleRepository) Titles(ctx context.Context, userID uuid.UUID) ([]*user.Title, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var titles []*user.Title
	// UUIDv7 grow with time: the last added is the newest.
	for _, t := range slices.Backward(r.titles) {
		if t.UserID() == userID {
			titles = append(titles, t)
		}
	}

	return titles, nil
}

func (r *TitleRepository) DeleteTitle(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	i := slices.IndexFunc(r.titles, func(t *user.Title) bool { return t.ID() == id })
	if i < 0 {
		return user.ErrTitleNotFound
	}

	r.titles = slices.Delete(r.titles, i, i+1)

	return nil
}
