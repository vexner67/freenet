package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// StickerPackRepository keeps the packs people make of their photos in
// custom_sticker_packs and custom_stickers.
type StickerPackRepository struct {
	pool *pgxpool.Pool
}

func NewStickerPackRepository(pool *pgxpool.Pool) *StickerPackRepository {
	return &StickerPackRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *StickerPackRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// lockAuthorQuery makes the packs of one author made in turn: the count
// below has to see the packs the others have made.
const lockAuthorQuery = `
	SELECT id
	FROM users
	WHERE id = $1
	FOR UPDATE
`

const createStickerPackQuery = `
	INSERT INTO custom_sticker_packs (
		id,
		author_id,
		name,
		about,
		price,
		published
	)
	SELECT
		$1,
		$2,
		$3,
		$4,
		$5,
		$6
	WHERE (
		SELECT count(*)
		FROM custom_sticker_packs
		WHERE author_id = $2
	) < $7
`

const insertStickersQuery = `
	INSERT INTO custom_stickers (
		photo_id,
		pack_id,
		position
	)
	SELECT
		photo,
		$1,
		ordinal - 1
	FROM unnest($2::uuid[]) WITH ORDINALITY AS s (photo, ordinal)
`

// Create has to run in a transaction (service.TxManager): the author's
// row stays locked until it ends.
func (r *StickerPackRepository) Create(ctx context.Context, p *user.CustomStickerPack) error {
	conn := r.conn(ctx)

	var locked uuid.UUID
	err := conn.QueryRow(ctx, lockAuthorQuery, p.AuthorID()).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("conn.QueryRow", err)
	}

	d := p.Draft()

	tag, err := conn.Exec(ctx, createStickerPackQuery,
		p.UUID(), p.AuthorID(), d.Name.String(), d.About.String(), d.Price, d.Published, user.MaxStickerPacks)
	if err != nil {
		return errs.Wrap("conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrTooManyStickerPacks
	}

	if _, err = conn.Exec(ctx, insertStickersQuery, p.UUID(), p.Photos()); err != nil {
		return errs.Wrap("conn.Exec", err)
	}

	return nil
}

// stickerPackColumns are what scanStickerPack reads, the stickers in the
// order they are shown.
const stickerPackColumns = `
		p.id,
		p.author_id,
		p.name,
		p.about,
		p.price,
		p.published,
		ARRAY(
			SELECT s.photo_id
			FROM custom_stickers s
			WHERE s.pack_id = p.id
			ORDER BY s.position
		)
`

const getStickerPackQuery = `
	SELECT` + stickerPackColumns + `
	FROM custom_sticker_packs p
	WHERE p.id = $1
	FOR SHARE OF p
`

const lockStickerPackQuery = `
	SELECT` + stickerPackColumns + `
	FROM custom_sticker_packs p
	WHERE p.id = $1
	FOR UPDATE OF p
`

func (r *StickerPackRepository) Get(ctx context.Context, id uuid.UUID) (*user.CustomStickerPack, error) {
	return r.one(ctx, getStickerPackQuery, id)
}

func (r *StickerPackRepository) Lock(ctx context.Context, id uuid.UUID) (*user.CustomStickerPack, error) {
	return r.one(ctx, lockStickerPackQuery, id)
}

func (r *StickerPackRepository) one(ctx context.Context, query string, id uuid.UUID) (*user.CustomStickerPack, error) {
	p, err := scanStickerPack(r.conn(ctx).QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrUnknownStickerPack
	}
	if err != nil {
		return nil, errs.Wrap("scanStickerPack", err)
	}

	return p, nil
}

const updateStickerPackQuery = `
	UPDATE custom_sticker_packs
	SET
		name = $2,
		about = $3,
		price = $4,
		published = $5
	WHERE id = $1
`

const deleteStickersQuery = `
	DELETE FROM custom_stickers
	WHERE pack_id = $1
`

// Update has to run in a transaction (service.TxManager), after Lock.
func (r *StickerPackRepository) Update(ctx context.Context, p *user.CustomStickerPack) error {
	conn := r.conn(ctx)
	d := p.Draft()

	if _, err := conn.Exec(ctx, updateStickerPackQuery, p.UUID(), d.Name.String(), d.About.String(), d.Price, d.Published); err != nil {
		return errs.Wrap("conn.Exec", err)
	}

	if _, err := conn.Exec(ctx, deleteStickersQuery, p.UUID()); err != nil {
		return errs.Wrap("conn.Exec", err)
	}

	if _, err := conn.Exec(ctx, insertStickersQuery, p.UUID(), p.Photos()); err != nil {
		return errs.Wrap("conn.Exec", err)
	}

	return nil
}

// The pack is in sticker_packs by the name of its ID.
const deleteStickerPackQuery = `
	WITH taken AS (
		DELETE FROM sticker_packs
		WHERE pack = replace($1::uuid::text, '-', '')
	)
	DELETE FROM custom_sticker_packs
	WHERE id = $1
`

func (r *StickerPackRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteStickerPackQuery, id); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const takenByOthersQuery = `
	SELECT EXISTS (
		SELECT 1
		FROM sticker_packs
		WHERE pack = $1
			AND user_id <> $2
	)
`

func (r *StickerPackRepository) TakenByOthers(ctx context.Context, p *user.CustomStickerPack) (bool, error) {
	var taken bool

	if err := r.conn(ctx).QueryRow(ctx, takenByOthersQuery, p.ID().String(), p.AuthorID()).Scan(&taken); err != nil {
		return false, errs.Wrap("r.conn.QueryRow", err)
	}

	return taken, nil
}

// A deleted page puts its packs out of the shop; those who took them keep
// them.
const visibleStickerPacksQuery = `
	SELECT` + stickerPackColumns + `
	FROM custom_sticker_packs p
	WHERE (
			p.published
			AND EXISTS (
				SELECT 1
				FROM users u
				WHERE u.id = p.author_id
					AND u.deactivated_at IS NULL
			)
		)
		OR p.author_id = $1
		OR EXISTS (
			SELECT 1
			FROM sticker_packs o
			WHERE o.user_id = $1
				AND o.pack = replace(p.id::text, '-', '')
		)
	ORDER BY p.id DESC
`

func (r *StickerPackRepository) Visible(ctx context.Context, viewerID uuid.UUID) ([]*user.CustomStickerPack, error) {
	rows, err := r.conn(ctx).Query(ctx, visibleStickerPacksQuery, viewerID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	packs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*user.CustomStickerPack, error) {
		return scanStickerPack(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return packs, nil
}

const authoredStickerPacksQuery = `
	SELECT` + stickerPackColumns + `,
		(
			SELECT count(*)
			FROM sticker_packs o
			WHERE o.pack = replace(p.id::text, '-', '')
				AND o.user_id <> p.author_id
		)
	FROM custom_sticker_packs p
	WHERE p.author_id = $1
	ORDER BY p.id DESC
`

func (r *StickerPackRepository) ByAuthor(ctx context.Context, authorID uuid.UUID) ([]service.AuthoredPack, error) {
	rows, err := r.conn(ctx).Query(ctx, authoredStickerPacksQuery, authorID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	packs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.AuthoredPack, error) {
		var a service.AuthoredPack

		p, err := scanStickerPack(row, &a.Holders)
		a.Pack = p

		return a, err
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return packs, nil
}

const stickerAuthorsQuery = `
	SELECT
		s.photo_id,
		p.author_id
	FROM custom_stickers s
	JOIN custom_sticker_packs p ON p.id = s.pack_id
	WHERE s.photo_id = ANY($1)
`

func (r *StickerPackRepository) Authors(ctx context.Context, photoIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, stickerAuthorsQuery, photoIDs)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	authors := make(map[uuid.UUID]uuid.UUID, len(photoIDs))

	for rows.Next() {
		var photo, author uuid.UUID
		if err = rows.Scan(&photo, &author); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		authors[photo] = author
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return authors, nil
}

// scanStickerPack reads stickerPackColumns and then what more there is.
func scanStickerPack(row pgx.Row, more ...any) (*user.CustomStickerPack, error) {
	var (
		id, author  uuid.UUID
		name, about string
		d           user.StickerPackDraft
		photos      []uuid.UUID
	)

	dest := append([]any{&id, &author, &name, &about, &d.Price, &d.Published, &photos}, more...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	d.Name, d.About = user.StickerPackName(name), user.StickerPackAbout(about)

	return user.RestoreCustomStickerPack(id, author, d, photos), nil
}
