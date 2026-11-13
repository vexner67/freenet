package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

type appUser struct {
	app, user uuid.UUID
}

// AppRepository keeps who added which HTML widget, over the documents it
// lists them from: a deleted widget is not listed.
type AppRepository struct {
	mu        sync.RWMutex
	installs  map[appUser]time.Time
	documents *DocumentRepository
}

func NewAppRepository(documents *DocumentRepository) *AppRepository {
	return &AppRepository{installs: make(map[appUser]time.Time), documents: documents}
}

func (r *AppRepository) Install(ctx context.Context, userID, app uuid.UUID, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.installs[appUser{app, userID}]; !ok {
		r.installs[appUser{app, userID}] = at
	}

	return nil
}

func (r *AppRepository) Uninstall(ctx context.Context, userID, app uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.installs, appUser{app, userID})

	return nil
}

func (r *AppRepository) List(ctx context.Context, section post.AppSection, userID, before uuid.UUID, limit int) ([]*post.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	r.documents.mu.RLock()
	defer r.documents.mu.RUnlock()

	var found []*post.Document
	for _, d := range r.documents.documents {
		if d.IsGame() && r.inSection(d, section, userID) && (before == uuid.Nil || bytes.Compare(idBytes(d), before[:]) < 0) {
			found = append(found, d)
		}
	}

	slices.SortFunc(found, func(a, b *post.Document) int { return bytes.Compare(idBytes(b), idBytes(a)) })
	if len(found) > limit {
		found = found[:limit]
	}

	return found, nil
}

// inSection tells whether d is in section of userID; the caller holds
// the locks.
func (r *AppRepository) inSection(d *post.Document, section post.AppSection, userID uuid.UUID) bool {
	switch section {
	case post.AppsOwn:
		return d.OwnerID() == userID
	case post.AppsInstalled:
		_, ok := r.installs[appUser{d.ID(), userID}]
		return ok
	default:
		for k := range r.installs {
			if k.app == d.ID() {
				return true
			}
		}
		return false
	}
}

func (r *AppRepository) Users(ctx context.Context, viewerID uuid.UUID, apps []uuid.UUID) (map[uuid.UUID]service.AppUsers, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	users := make(map[uuid.UUID]service.AppUsers, len(apps))
	for _, app := range apps {
		var u service.AppUsers
		for k := range r.installs {
			if k.app == app {
				u.Count++
				u.Installed = u.Installed || (viewerID != uuid.Nil && k.user == viewerID)
			}
		}
		users[app] = u
	}

	return users, nil
}

func (r *AppRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for k := range r.installs {
		if k.user == userID {
			delete(r.installs, k)
		}
	}

	return nil
}
