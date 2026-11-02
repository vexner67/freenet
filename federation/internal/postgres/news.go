package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

type FollowingRepository struct {
	pool *pgxpool.Pool
}

func NewFollowingRepository(pool *pgxpool.Pool) *FollowingRepository {
	return &FollowingRepository{
		pool: pool,
	}
}

func (r *FollowingRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const getFollowingQuery = `
	SELECT
		actor,
		follow_id,
		accepted,
		featured,
		created_at
	FROM following
	WHERE actor = $1
`

func (r *FollowingRepository) Get(ctx context.Context, actor fedi.URL) (*fedi.Following, error) {
	f, err := scanFollowing(r.conn(ctx).QueryRow(ctx, getFollowingQuery, actor.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fedi.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanFollowing", err)
	}

	return f, nil
}

func scanFollowing(row pgx.Row) (*fedi.Following, error) {
	var (
		actor              string
		followID           uuid.UUID
		accepted, featured bool
		createdAt          time.Time
	)

	if err := row.Scan(&actor, &followID, &accepted, &featured, &createdAt); err != nil {
		return nil, err
	}

	return fedi.RestoreFollowing(fedi.URL(actor), followID, accepted, featured, createdAt), nil
}

const saveFollowingQuery = `
	INSERT INTO following (
		actor,
		follow_id,
		accepted,
		featured,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (actor) DO UPDATE
	SET accepted = EXCLUDED.accepted,
		featured = EXCLUDED.featured
`

func (r *FollowingRepository) Save(ctx context.Context, f *fedi.Following) error {
	_, err := r.conn(ctx).Exec(ctx, saveFollowingQuery, f.Actor().String(), f.FollowID(), f.Accepted(), f.Featured(), f.CreatedAt())
	if err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const deleteFollowingQuery = `
	DELETE FROM following
	WHERE actor = $1
`

func (r *FollowingRepository) Delete(ctx context.Context, actor fedi.URL) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteFollowingQuery, actor.String()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const listFeaturedQuery = `
	SELECT
		actor,
		follow_id,
		accepted,
		featured,
		created_at
	FROM following
	WHERE featured
	ORDER BY created_at
`

func (r *FollowingRepository) Featured(ctx context.Context) ([]*fedi.Following, error) {
	rows, err := r.conn(ctx).Query(ctx, listFeaturedQuery)
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}

	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*fedi.Following, error) {
		return scanFollowing(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return out, nil
}

// listChannelsQuery: the account of a channel of Telegram is its address
// there, https://t.me/<name>.
const listChannelsQuery = `
	SELECT
		actor
	FROM following
	WHERE actor LIKE 'https://t.me/%'
	ORDER BY actor
`

func (r *FollowingRepository) Channels(ctx context.Context) ([]fedi.URL, error) {
	rows, err := r.conn(ctx).Query(ctx, listChannelsQuery)
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}

	out, err := pgx.CollectRows(rows, pgx.RowTo[fedi.URL])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return out, nil
}

type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

func NewSubscriptionRepository(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{
		pool: pool,
	}
}

func (r *SubscriptionRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const addSubscriptionQuery = `
	INSERT INTO subscriptions (
		user_id,
		actor,
		created_at
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (user_id, actor) DO NOTHING
`

func (r *SubscriptionRepository) Add(ctx context.Context, userID uuid.UUID, actor fedi.URL, now time.Time) error {
	if _, err := r.conn(ctx).Exec(ctx, addSubscriptionQuery, userID, actor.String(), now.UTC()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const removeSubscriptionQuery = `
	DELETE FROM subscriptions
	WHERE user_id = $1
		AND actor = $2
`

func (r *SubscriptionRepository) Remove(ctx context.Context, userID uuid.UUID, actor fedi.URL) error {
	if _, err := r.conn(ctx).Exec(ctx, removeSubscriptionQuery, userID, actor.String()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const removeSubscribedActorQuery = `
	DELETE FROM subscriptions
	WHERE actor = $1
`

func (r *SubscriptionRepository) RemoveActor(ctx context.Context, actor fedi.URL) error {
	if _, err := r.conn(ctx).Exec(ctx, removeSubscribedActorQuery, actor.String()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const listSubscriptionsQuery = `
	SELECT actor
	FROM subscriptions
	WHERE user_id = $1
	ORDER BY created_at DESC
`

func (r *SubscriptionRepository) Of(ctx context.Context, userID uuid.UUID) ([]fedi.URL, error) {
	rows, err := r.conn(ctx).Query(ctx, listSubscriptionsQuery, userID)
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}

	actors, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (fedi.URL, error) {
		var actor string
		err := row.Scan(&actor)
		return fedi.URL(actor), err
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return actors, nil
}

const actorReadQuery = `
	SELECT EXISTS (
		SELECT 1
		FROM subscriptions
		WHERE actor = $1
	)
`

func (r *SubscriptionRepository) Read(ctx context.Context, actor fedi.URL) (bool, error) {
	var read bool
	if err := r.conn(ctx).QueryRow(ctx, actorReadQuery, actor.String()).Scan(&read); err != nil {
		return false, errs.Wrap("QueryRow", err)
	}

	return read, nil
}

type RemotePostRepository struct {
	pool *pgxpool.Pool
}

func NewRemotePostRepository(pool *pgxpool.Pool) *RemotePostRepository {
	return &RemotePostRepository{
		pool: pool,
	}
}

func (r *RemotePostRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// imageJSON is a picture of a remote post in the images column.
type imageJSON struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// videoJSON is a video of a remote post in the videos column.
type videoJSON struct {
	URL         string        `json:"url"`
	Cover       string        `json:"cover,omitempty"`
	Blurhash    string        `json:"blurhash,omitempty"`
	Width       int           `json:"width,omitempty"`
	Height      int           `json:"height,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	Description string        `json:"description,omitempty"`
}

// linkJSON is the card of the page a remote post links to, in the link
// column; NULL for none.
type linkJSON struct {
	URL         string `json:"url"`
	Site        string `json:"site,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
}

const remotePostColumns = `
		p.id,
		p.uri,
		p.actor,
		p.page,
		p.text,
		p.summary,
		p.images,
		p.videos,
		p.sensitive,
		p.language,
		p.published,
		p.updated,
		p.link
`

func scanRemotePost(row pgx.Row) (*fedi.RemotePost, error) {
	var (
		id                                   uuid.UUID
		uri, actor, page, text, summary, lng string
		imagesJSON, videosJSON, cardJSON     []byte
		sensitive                            bool
		published                            time.Time
		updated                              *time.Time
	)

	if err := row.Scan(&id, &uri, &actor, &page, &text, &summary, &imagesJSON, &videosJSON, &sensitive, &lng, &published, &updated, &cardJSON); err != nil {
		return nil, err
	}

	var stored []imageJSON
	if err := json.Unmarshal(imagesJSON, &stored); err != nil {
		return nil, errs.Wrap("json.Unmarshal", err)
	}

	images := make([]fedi.Image, len(stored))
	for i, im := range stored {
		images[i] = fedi.Image{URL: fedi.URL(im.URL), Description: im.Description}
	}

	videos, err := scanVideos(videosJSON)
	if err != nil {
		return nil, err
	}

	var edited time.Time
	if updated != nil {
		edited = *updated
	}

	var card fedi.Link
	if cardJSON != nil {
		var stored linkJSON
		if err := json.Unmarshal(cardJSON, &stored); err != nil {
			return nil, errs.Wrap("json.Unmarshal", err)
		}
		card = fedi.Link{
			URL: fedi.URL(stored.URL), Site: stored.Site, Title: stored.Title, Description: stored.Description, Image: fedi.URL(stored.Image),
		}
	}

	return fedi.RestoreRemotePost(id, fedi.URL(uri), fedi.URL(actor), fedi.URL(page), text, summary, images, videos, sensitive, lng, published, edited).
		WithLink(card), nil
}

// cardJSON is the link column of a card: NULL for none.
func cardJSON(l fedi.Link) ([]byte, error) {
	if l.URL == "" {
		return nil, nil
	}

	data, err := json.Marshal(linkJSON{
		URL: l.URL.String(), Site: l.Site, Title: l.Title, Description: l.Description, Image: l.Image.String(),
	})
	return data, errs.Wrap("json.Marshal", err)
}

func scanVideos(data []byte) ([]fedi.Video, error) {
	var stored []videoJSON
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, errs.Wrap("json.Unmarshal", err)
	}

	videos := make([]fedi.Video, len(stored))
	for i, v := range stored {
		videos[i] = fedi.Video{
			URL: fedi.URL(v.URL), Cover: fedi.URL(v.Cover), Blurhash: v.Blurhash,
			Width: v.Width, Height: v.Height, Duration: v.Duration, Description: v.Description,
		}
	}

	return videos, nil
}

func videosJSON(videos []fedi.Video) ([]byte, error) {
	stored := make([]videoJSON, len(videos))
	for i, v := range videos {
		stored[i] = videoJSON{
			URL: v.URL.String(), Cover: v.Cover.String(), Blurhash: v.Blurhash,
			Width: v.Width, Height: v.Height, Duration: v.Duration, Description: v.Description,
		}
	}

	data, err := json.Marshal(stored)
	return data, errs.Wrap("json.Marshal", err)
}

const getRemotePostQuery = `
	SELECT` + remotePostColumns + `
	FROM remote_posts p
	WHERE p.id = $1
`

func (r *RemotePostRepository) Get(ctx context.Context, id uuid.UUID) (*fedi.RemotePost, error) {
	return r.one(ctx, getRemotePostQuery, id)
}

const getRemotePostByURIQuery = `
	SELECT` + remotePostColumns + `
	FROM remote_posts p
	WHERE p.uri = $1
`

func (r *RemotePostRepository) ByURI(ctx context.Context, uri fedi.URL) (*fedi.RemotePost, error) {
	return r.one(ctx, getRemotePostByURIQuery, uri.String())
}

func (r *RemotePostRepository) one(ctx context.Context, query string, arg any) (*fedi.RemotePost, error) {
	p, err := scanRemotePost(r.conn(ctx).QueryRow(ctx, query, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fedi.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanRemotePost", err)
	}

	return p, nil
}

const saveRemotePostQuery = `
	INSERT INTO remote_posts (
		id,
		uri,
		actor,
		page,
		text,
		summary,
		images,
		videos,
		sensitive,
		language,
		published,
		updated,
		link
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	ON CONFLICT (uri) DO UPDATE
	SET page = EXCLUDED.page,
		text = EXCLUDED.text,
		summary = EXCLUDED.summary,
		images = EXCLUDED.images,
		videos = EXCLUDED.videos,
		sensitive = EXCLUDED.sensitive,
		language = EXCLUDED.language,
		updated = EXCLUDED.updated,
		link = EXCLUDED.link
`

func (r *RemotePostRepository) Save(ctx context.Context, p *fedi.RemotePost) error {
	images := make([]imageJSON, 0, len(p.Images()))
	for _, im := range p.Images() {
		images = append(images, imageJSON{URL: im.URL.String(), Description: im.Description})
	}

	imagesJSON, err := json.Marshal(images)
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	videos, err := videosJSON(p.Videos())
	if err != nil {
		return err
	}

	card, err := cardJSON(p.Link())
	if err != nil {
		return err
	}

	var updated *time.Time
	if t := p.Updated(); !t.IsZero() {
		updated = &t
	}

	_, err = r.conn(ctx).Exec(ctx, saveRemotePostQuery,
		p.ID(), p.URI().String(), p.Actor().String(), p.Page().String(), p.Text(), p.Summary(), imagesJSON, videos,
		p.Sensitive(), p.Language(), p.Published(), updated, card)
	if err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const deleteRemotePostQuery = `
	DELETE FROM remote_posts
	WHERE uri = $1
`

func (r *RemotePostRepository) Delete(ctx context.Context, uri fedi.URL) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteRemotePostQuery, uri.String()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const deleteActorPostsQuery = `
	DELETE FROM remote_posts
	WHERE actor = $1
`

func (r *RemotePostRepository) DeleteActor(ctx context.Context, actor fedi.URL) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteActorPostsQuery, actor.String()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

// The news of everyone: the featured accounts, in the languages; $2 is
// whether sensitive posts are left out, $3 the cursor or NULL.
const listFeaturedNewsQuery = `
	SELECT` + remotePostColumns + `
	FROM remote_posts p
	JOIN following f ON f.actor = p.actor AND f.featured
	WHERE (p.language = '' OR p.language = ANY($1))
		AND NOT ($2 AND p.sensitive)
		AND ($3::uuid IS NULL OR p.id < $3)
	ORDER BY p.id DESC
	LIMIT $4
`

// The news of a reader: the accounts they subscribed to.
const listSubscribedNewsQuery = `
	SELECT` + remotePostColumns + `
	FROM remote_posts p
	JOIN subscriptions s ON s.actor = p.actor AND s.user_id = $1
	WHERE NOT ($2 AND p.sensitive)
		AND ($3::uuid IS NULL OR p.id < $3)
	ORDER BY p.id DESC
	LIMIT $4
`

func (r *RemotePostRepository) List(ctx context.Context, q service.NewsQuery) ([]*fedi.RemotePost, error) {
	var before *uuid.UUID
	if q.Before != uuid.Nil {
		before = &q.Before
	}

	query, first := listSubscribedNewsQuery, any(q.Reader)
	if q.Featured {
		query, first = listFeaturedNewsQuery, q.Languages
	}

	rows, err := r.conn(ctx).Query(ctx, query, first, q.HideSensitive, before, q.Limit)
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}

	posts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*fedi.RemotePost, error) {
		return scanRemotePost(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return posts, nil
}

const listActorPostsQuery = `
	SELECT uri
	FROM remote_posts
	WHERE actor = $1
`

func (r *RemotePostRepository) URIsOf(ctx context.Context, actor fedi.URL) ([]fedi.URL, error) {
	rows, err := r.conn(ctx).Query(ctx, listActorPostsQuery, actor.String())
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}

	uris, err := pgx.CollectRows(rows, pgx.RowTo[fedi.URL])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return uris, nil
}

const deleteOldPostsQuery = `
	DELETE FROM remote_posts
	WHERE published < $1
`

func (r *RemotePostRepository) DeleteBefore(ctx context.Context, t time.Time) (int, error) {
	tag, err := r.conn(ctx).Exec(ctx, deleteOldPostsQuery, t.UTC())
	if err != nil {
		return 0, errs.Wrap("Exec", err)
	}

	return int(tag.RowsAffected()), nil
}

type CoverRepository struct {
	pool *pgxpool.Pool
}

func NewCoverRepository(pool *pgxpool.Pool) *CoverRepository {
	return &CoverRepository{
		pool: pool,
	}
}

const getCoverQuery = `
	SELECT frame
	FROM remote_video_covers
	WHERE post_id = $1
		AND video = $2
`

func (r *CoverRepository) Get(ctx context.Context, postID uuid.UUID, video fedi.URL) ([]byte, error) {
	var frame []byte

	err := r.pool.QueryRow(ctx, getCoverQuery, postID, video.String()).Scan(&frame)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fedi.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("Scan", err)
	}

	return frame, nil
}

// saveCoverQuery keeps nothing for a post deleted while its frame was cut.
const saveCoverQuery = `
	INSERT INTO remote_video_covers (
		post_id,
		video,
		frame
	)
	SELECT $1, $2, $3
	WHERE EXISTS (SELECT 1 FROM remote_posts WHERE id = $1)
	ON CONFLICT (post_id, video) DO UPDATE
	SET frame = EXCLUDED.frame
`

func (r *CoverRepository) Save(ctx context.Context, postID uuid.UUID, video fedi.URL, frame []byte) error {
	if frame == nil {
		frame = []byte{}
	}

	if _, err := r.pool.Exec(ctx, saveCoverQuery, postID, video.String(), frame); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}
