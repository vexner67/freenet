// Package postgres keeps the accounts, followers and remote actors of
// federation in PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

type AccountRepository struct {
	pool *pgxpool.Pool
}

func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{
		pool: pool,
	}
}

func (r *AccountRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const getAccountQuery = `
	SELECT
		public_key,
		private_key,
		open,
		created_at
	FROM accounts
	WHERE user_id = $1
`

func (r *AccountRepository) Get(ctx context.Context, userID uuid.UUID) (*fedi.Account, error) {
	var (
		keys      fedi.KeyPair
		open      bool
		createdAt time.Time
	)

	err := r.conn(ctx).QueryRow(ctx, getAccountQuery, userID).Scan(&keys.Public, &keys.Private, &open, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fedi.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("QueryRow", err)
	}

	return fedi.RestoreAccount(userID, keys, open, createdAt), nil
}

const saveAccountQuery = `
	INSERT INTO accounts (
		user_id,
		public_key,
		private_key,
		open,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (user_id) DO UPDATE
	SET open = EXCLUDED.open
`

// Save keeps a new account, or whether a known one is open: the keys of
// an account never change.
func (r *AccountRepository) Save(ctx context.Context, a *fedi.Account) error {
	keys := a.Keys()

	if _, err := r.conn(ctx).Exec(ctx, saveAccountQuery, a.UserID(), keys.Public, keys.Private, a.Open(), a.CreatedAt()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

type FollowerRepository struct {
	pool *pgxpool.Pool
}

func NewFollowerRepository(pool *pgxpool.Pool) *FollowerRepository {
	return &FollowerRepository{
		pool: pool,
	}
}

func (r *FollowerRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const addFollowerQuery = `
	INSERT INTO followers (
		user_id,
		actor,
		inbox,
		created_at
	)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (user_id, actor) DO UPDATE
	SET inbox = EXCLUDED.inbox
`

func (r *FollowerRepository) Add(ctx context.Context, f *fedi.Follower) error {
	if _, err := r.conn(ctx).Exec(ctx, addFollowerQuery, f.UserID(), f.Actor().String(), f.Inbox().String(), f.CreatedAt()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const removeFollowerQuery = `
	DELETE FROM followers
	WHERE user_id = $1
		AND actor = $2
`

func (r *FollowerRepository) Remove(ctx context.Context, userID uuid.UUID, actor fedi.URL) error {
	if _, err := r.conn(ctx).Exec(ctx, removeFollowerQuery, userID, actor.String()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const removeActorQuery = `
	DELETE FROM followers
	WHERE actor = $1
`

func (r *FollowerRepository) RemoveActor(ctx context.Context, actor fedi.URL) error {
	if _, err := r.conn(ctx).Exec(ctx, removeActorQuery, actor.String()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const removeAllFollowersQuery = `
	DELETE FROM followers
	WHERE user_id = $1
`

func (r *FollowerRepository) RemoveAll(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, removeAllFollowersQuery, userID); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const listInboxesQuery = `
	SELECT DISTINCT inbox
	FROM followers
	WHERE user_id = $1
	ORDER BY inbox
`

func (r *FollowerRepository) Inboxes(ctx context.Context, userID uuid.UUID) ([]fedi.URL, error) {
	rows, err := r.conn(ctx).Query(ctx, listInboxesQuery, userID)
	if err != nil {
		return nil, errs.Wrap("Query", err)
	}

	inboxes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (fedi.URL, error) {
		var inbox string
		err := row.Scan(&inbox)
		return fedi.URL(inbox), err
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return inboxes, nil
}

const countFollowersQuery = `
	SELECT count(*)
	FROM followers
	WHERE user_id = $1
`

func (r *FollowerRepository) Count(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	if err := r.conn(ctx).QueryRow(ctx, countFollowersQuery, userID).Scan(&n); err != nil {
		return 0, errs.Wrap("QueryRow", err)
	}

	return n, nil
}

type RemoteRepository struct {
	pool *pgxpool.Pool
}

func NewRemoteRepository(pool *pgxpool.Pool) *RemoteRepository {
	return &RemoteRepository{
		pool: pool,
	}
}

func (r *RemoteRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const getRemoteQuery = `
	SELECT
		inbox,
		shared_inbox,
		key_id,
		public_key,
		name,
		username,
		icon,
		page,
		fetched_at
	FROM remote_actors
	WHERE id = $1
`

func (r *RemoteRepository) Get(ctx context.Context, id fedi.URL) (*fedi.Remote, error) {
	var (
		inbox, shared, keyID, publicKey string
		icon, page                      string
		profile                         fedi.Profile
		fetchedAt                       time.Time
	)

	err := r.conn(ctx).QueryRow(ctx, getRemoteQuery, id.String()).Scan(
		&inbox, &shared, &keyID, &publicKey, &profile.Name, &profile.Username, &icon, &page, &fetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fedi.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("QueryRow", err)
	}

	profile.Icon, profile.Page = fedi.URL(icon), fedi.URL(page)

	return fedi.RestoreRemote(id, fedi.URL(inbox), fedi.URL(shared), fedi.URL(keyID), publicKey, profile, fetchedAt), nil
}

const saveRemoteQuery = `
	INSERT INTO remote_actors (
		id,
		inbox,
		shared_inbox,
		key_id,
		public_key,
		name,
		username,
		icon,
		page,
		fetched_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	ON CONFLICT (id) DO UPDATE
	SET inbox = EXCLUDED.inbox,
		shared_inbox = EXCLUDED.shared_inbox,
		key_id = EXCLUDED.key_id,
		public_key = EXCLUDED.public_key,
		name = EXCLUDED.name,
		username = EXCLUDED.username,
		icon = EXCLUDED.icon,
		page = EXCLUDED.page,
		fetched_at = EXCLUDED.fetched_at
`

func (r *RemoteRepository) Save(ctx context.Context, a *fedi.Remote) error {
	p := a.Profile()

	_, err := r.conn(ctx).Exec(ctx, saveRemoteQuery,
		a.ID().String(), a.Inbox().String(), a.SharedInbox().String(), a.KeyID().String(), a.PublicKey(),
		p.Name, p.Username, p.Icon.String(), p.Page.String(), a.FetchedAt())
	if err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}

const deleteRemoteQuery = `
	DELETE FROM remote_actors
	WHERE id = $1
`

func (r *RemoteRepository) Delete(ctx context.Context, id fedi.URL) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteRemoteQuery, id.String()); err != nil {
		return errs.Wrap("Exec", err)
	}

	return nil
}
