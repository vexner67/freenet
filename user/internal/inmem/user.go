package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

type UserRepository struct {
	mu    sync.RWMutex
	users map[uuid.UUID]*user.User
}

func NewUserRepository() *UserRepository {
	return &UserRepository{
		users: make(map[uuid.UUID]*user.User),
	}
}

func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	select {
	case <-ctx.Done():
		return errs.Wrap("ctx.Done", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.users[u.ID()] = u

	return nil
}
