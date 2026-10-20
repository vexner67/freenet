package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// AdRepository keeps the ads of the global feed.
type AdRepository struct {
	pool *pgxpool.Pool
}

func NewAdRepository(pool *pgxpool.Pool) *AdRepository {
	return &AdRepository{
		pool: pool,
	}
}

func (r *AdRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// The count of ads is checked by the statement that adds one; two added
// at once may pass it both, one ad over the limit.
const saveAdQuery = `
	INSERT INTO ads (
		id,
		url,
		title,
		body,
		photo_owner_id,
		photo_id,
		active
	)
	SELECT $1, $2, $3, $4, $5, $6, $7
	WHERE (SELECT count(*) FROM ads) < $8
`

func (r *AdRepository) Save(ctx context.Context, ad *user.Ad) error {
	owner, photo := photoColumns(ad.Photo())

	tag, err := r.conn(ctx).Exec(ctx, saveAdQuery,
		ad.ID(), ad.URL().String(), ad.Title().String(), ad.Text().String(), owner, photo, ad.IsActive(), user.MaxAds)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrTooManyAds
	}

	return nil
}

const lockAdQuery = `
	SELECT
		id,
		url,
		title,
		body,
		photo_owner_id,
		photo_id,
		active
	FROM ads
	WHERE id = $1
	FOR UPDATE
`

func (r *AdRepository) Lock(ctx context.Context, id uuid.UUID) (*user.Ad, error) {
	ad, err := scanAd(r.conn(ctx).QueryRow(ctx, lockAdQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrAdNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanAd", err)
	}

	return ad, nil
}

const updateAdQuery = `
	UPDATE ads
	SET url = $2,
		title = $3,
		body = $4,
		photo_owner_id = $5,
		photo_id = $6,
		active = $7
	WHERE id = $1
`

func (r *AdRepository) Update(ctx context.Context, ad *user.Ad) error {
	owner, photo := photoColumns(ad.Photo())

	tag, err := r.conn(ctx).Exec(ctx, updateAdQuery,
		ad.ID(), ad.URL().String(), ad.Title().String(), ad.Text().String(), owner, photo, ad.IsActive())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrAdNotFound
	}

	return nil
}

const deleteAdQuery = `
	DELETE FROM ads
	WHERE id = $1
	RETURNING
		id,
		url,
		title,
		body,
		photo_owner_id,
		photo_id,
		active
`

func (r *AdRepository) Delete(ctx context.Context, id uuid.UUID) (*user.Ad, error) {
	ad, err := scanAd(r.conn(ctx).QueryRow(ctx, deleteAdQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrAdNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanAd", err)
	}

	return ad, nil
}

const listAdsQuery = `
	SELECT
		id,
		url,
		title,
		body,
		photo_owner_id,
		photo_id,
		active
	FROM ads
	WHERE active OR $1
	ORDER BY id DESC
`

func (r *AdRepository) List(ctx context.Context, all bool) ([]*user.Ad, error) {
	rows, err := r.conn(ctx).Query(ctx, listAdsQuery, all)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ads, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*user.Ad, error) {
		return scanAd(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ads, nil
}

func scanAd(row pgx.Row) (*user.Ad, error) {
	var (
		id           uuid.UUID
		url, title   string
		body         string
		owner, photo *uuid.UUID
		active       bool
	)

	if err := row.Scan(&id, &url, &title, &body, &owner, &photo, &active); err != nil {
		return nil, err
	}

	d := user.AdDraft{URL: user.AdURL(url), Title: user.AdTitle(title), Text: user.AdText(body), Active: active}
	if photo != nil && owner != nil {
		d.Photo = user.AdPhoto{OwnerID: *owner, PhotoID: *photo}
	}

	return user.RestoreAd(id, d), nil
}

// photoColumns are the cover as the table keeps it: two NULLs for none.
func photoColumns(p user.AdPhoto) (owner, photo *uuid.UUID) {
	if p.IsZero() {
		return nil, nil
	}

	return &p.OwnerID, &p.PhotoID
}

// A reader's views and clicks of an ad are one row, so the stats know how
// many people saw and followed it; an ad that is not shown counts nothing.
const recordAdQuery = `
	INSERT INTO ad_stats (
		ad_id,
		user_id,
		views,
		clicks
	)
	SELECT id, $2, $3, $4
	FROM ads
	WHERE id = $1
		AND active
	ON CONFLICT (ad_id, user_id) DO UPDATE
	SET views = ad_stats.views + EXCLUDED.views,
		clicks = ad_stats.clicks + EXCLUDED.clicks
`

func (r *AdRepository) Record(ctx context.Context, adID, readerID uuid.UUID, event user.AdEvent) error {
	var views, clicks int
	switch event {
	case user.AdViewed:
		views = 1
	case user.AdClicked:
		clicks = 1
	}

	tag, err := r.conn(ctx).Exec(ctx, recordAdQuery, adID, readerID, views, clicks)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrAdNotFound
	}

	return nil
}

const adStatsQuery = `
	SELECT
		ad_id,
		sum(views)::bigint,
		count(*) FILTER (WHERE views > 0),
		sum(clicks)::bigint,
		count(*) FILTER (WHERE clicks > 0)
	FROM ad_stats
	GROUP BY ad_id
`

func (r *AdRepository) Stats(ctx context.Context) (map[uuid.UUID]user.AdStats, error) {
	rows, err := r.conn(ctx).Query(ctx, adStatsQuery)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	stats := make(map[uuid.UUID]user.AdStats)
	for rows.Next() {
		var (
			id uuid.UUID
			s  user.AdStats
		)

		if err = rows.Scan(&id, &s.Views, &s.Viewers, &s.Clicks, &s.Clickers); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		stats[id] = s
	}

	return stats, errs.Wrap("rows.Err", rows.Err())
}
