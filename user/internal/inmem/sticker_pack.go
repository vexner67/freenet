package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// StickerPackRepository keeps the packs people make in memory; they go
// with their author, as in the table. Without transactions, Get and Lock
// hold nothing.
type StickerPackRepository struct {
	mu       sync.Mutex
	packs    map[uuid.UUID]*user.CustomStickerPack
	users    *UserRepository
	stickers *StickerRepository
}

func NewStickerPackRepository(users *UserRepository, stickers *StickerRepository) *StickerPackRepository {
	r := &StickerPackRepository{
		packs:    make(map[uuid.UUID]*user.CustomStickerPack),
		users:    users,
		stickers: stickers,
	}
	users.onDelete(r.forget)

	return r
}

func (r *StickerPackRepository) forget(authorID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, p := range r.packs {
		if p.AuthorID() == authorID {
			delete(r.packs, id)
		}
	}
}

func (r *StickerPackRepository) Create(ctx context.Context, p *user.CustomStickerPack) error {
	if _, err := r.users.GetByID(ctx, p.AuthorID()); err != nil {
		return errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	made := 0
	for _, other := range r.packs {
		if other.AuthorID() == p.AuthorID() {
			made++
		}
	}

	if made >= user.MaxStickerPacks {
		return user.ErrTooManyStickerPacks
	}

	r.packs[p.UUID()] = p

	return nil
}

func (r *StickerPackRepository) Get(ctx context.Context, id uuid.UUID) (*user.CustomStickerPack, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.packs[id]
	if !ok {
		return nil, user.ErrUnknownStickerPack
	}

	return p, nil
}

func (r *StickerPackRepository) Lock(ctx context.Context, id uuid.UUID) (*user.CustomStickerPack, error) {
	return r.Get(ctx, id)
}

func (r *StickerPackRepository) Update(ctx context.Context, p *user.CustomStickerPack) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.packs[p.UUID()] = p

	return nil
}

func (r *StickerPackRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	p, ok := r.packs[id]
	delete(r.packs, id)
	r.mu.Unlock()

	if ok {
		r.stickers.drop(p.ID())
	}

	return nil
}

func (r *StickerPackRepository) TakenByOthers(ctx context.Context, p *user.CustomStickerPack) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	return slices.ContainsFunc(r.stickers.holders(p.ID()), func(id uuid.UUID) bool { return id != p.AuthorID() }), nil
}

func (r *StickerPackRepository) Visible(ctx context.Context, viewerID uuid.UUID) ([]*user.CustomStickerPack, error) {
	var visible []*user.CustomStickerPack

	for _, p := range r.all() {
		shown := p.Draft().Published && r.active(ctx, p.AuthorID())
		if shown || p.AuthorID() == viewerID || slices.Contains(r.stickers.holders(p.ID()), viewerID) {
			visible = append(visible, p)
		}
	}

	return visible, nil
}

func (r *StickerPackRepository) active(ctx context.Context, userID uuid.UUID) bool {
	u, err := r.users.GetByID(ctx, userID)

	return err == nil && !u.IsDeactivated()
}

func (r *StickerPackRepository) ByAuthor(ctx context.Context, authorID uuid.UUID) ([]service.AuthoredPack, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	var authored []service.AuthoredPack

	for _, p := range r.all() {
		if p.AuthorID() != authorID {
			continue
		}

		holders := slices.DeleteFunc(r.stickers.holders(p.ID()), func(id uuid.UUID) bool { return id == authorID })
		authored = append(authored, service.AuthoredPack{Pack: p, Holders: len(holders)})
	}

	return authored, nil
}

func (r *StickerPackRepository) Authors(ctx context.Context, photoIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	authors := make(map[uuid.UUID]uuid.UUID, len(photoIDs))

	for _, p := range r.all() {
		for _, photo := range p.Photos() {
			if slices.Contains(photoIDs, photo) {
				authors[photo] = p.AuthorID()
			}
		}
	}

	return authors, nil
}

// all are the packs, newest first.
func (r *StickerPackRepository) all() []*user.CustomStickerPack {
	r.mu.Lock()
	defer r.mu.Unlock()

	packs := make([]*user.CustomStickerPack, 0, len(r.packs))
	for _, p := range r.packs {
		packs = append(packs, p)
	}

	slices.SortFunc(packs, func(a, b *user.CustomStickerPack) int {
		id := a.UUID()
		other := b.UUID()

		return bytes.Compare(other[:], id[:])
	})

	return packs
}
