package inmem

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
)

type Repository struct {
	mu    sync.Mutex
	rooms map[uuid.UUID]*room.Room
	bans  map[[2]uuid.UUID]time.Time
}

func NewRepository() *Repository {
	return &Repository{rooms: map[uuid.UUID]*room.Room{}, bans: map[[2]uuid.UUID]time.Time{}}
}
func clone(r *room.Room) *room.Room {
	return room.Restore(r.ID(), room.Input{OwnerID: r.OwnerID(), ClubID: r.ClubID(), Title: r.Title(), Kind: r.Kind(), Access: r.Access(), Control: r.Control(), Permanent: r.Permanent()}, r.CreatedAt(), r.ClosedAt())
}
func (r *Repository) Get(_ context.Context, id uuid.UUID) (*room.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	found := r.rooms[id]
	if found == nil {
		return nil, room.ErrNotFound
	}
	return clone(found), nil
}
func (r *Repository) Save(_ context.Context, value *room.Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.rooms[value.ID()]; old != nil && old.Closed() && !value.Closed() {
		return room.ErrClosed
	}
	r.rooms[value.ID()] = clone(value)
	return nil
}
func (r *Repository) List(_ context.Context, f service.Filter) ([]*room.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := make([]*room.Room, 0)
	for _, value := range r.rooms {
		if (!f.IncludeClosed && value.Closed()) || (f.PersonalOnly && value.ClubID() != uuid.Nil) || (f.OwnerID != uuid.Nil && value.OwnerID() != f.OwnerID) || (f.ClubID != uuid.Nil && value.ClubID() != f.ClubID) || (f.PermanentOnly && !value.Permanent()) || (f.Cursor != uuid.Nil && value.ID().String() >= f.Cursor.String()) {
			continue
		}
		rows = append(rows, clone(value))
	}
	slices.SortFunc(rows, func(a, b *room.Room) int { return -strings.Compare(a.ID().String(), b.ID().String()) })
	if f.Limit > 0 && len(rows) > f.Limit {
		rows = rows[:f.Limit]
	}
	return rows, nil
}
func (r *Repository) Until(_ context.Context, id, user uuid.UUID) (time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bans[[2]uuid.UUID{id, user}], nil
}
func (r *Repository) SaveBan(_ context.Context, id, user uuid.UUID, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bans[[2]uuid.UUID{id, user}] = until
	return nil
}

type txKey struct{}
type TxManager struct{ mu sync.Mutex }

func (t *TxManager) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	if ctx.Value(txKey{}) == t {
		return fn(ctx)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(context.WithValue(ctx, txKey{}, t))
}

func (r *Repository) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rooms, id)
	for key := range r.bans {
		if key[0] == id {
			delete(r.bans, key)
		}
	}
	return nil
}
func (r *Repository) DeleteUser(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.bans {
		if key[1] == id {
			delete(r.bans, key)
		}
	}
	return nil
}
