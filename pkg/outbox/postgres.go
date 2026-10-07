package outbox

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/reqid"
)

const (
	channel      = "outbox"
	closeTimeout = 5 * time.Second
)

// Postgres is the outbox table of a service, which its migrations create:
//
//	CREATE TABLE outbox (
//	    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
//	    stream text NOT NULL,
//	    kind text NOT NULL,
//	    payload bytea NOT NULL,
//	    request_id text NOT NULL DEFAULT '',
//	    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
//	    due_at timestamptz NOT NULL DEFAULT now(),
//	    created_at timestamptz NOT NULL DEFAULT now()
//	);
//	CREATE INDEX outbox_stream_idx ON outbox (stream, id);
//
// A table holds several streams, one per receiver: each has its own order
// and relay, so a receiver that is down holds back only its own messages.
//
// The order is that of the identity, given out by the database: clocks of
// replicas do not matter. The relay that holds the advisory lock of the
// stream on its own connection delivers; it learns about new messages by LISTEN, and as a
// NOTIFY arrives only once its transaction commits, it never wakes up
// for a message it cannot see yet.
type Postgres struct {
	pool   *pgxpool.Pool
	stream string
	// lockKey is the advisory lock of the stream's relay.
	lockKey int64
	// conn holds the lock and listens; the relay owns it.
	conn   *pgx.Conn
	leader bool
}

// NewPostgres is the stream of the outbox table, e.g. the messages for one
// receiving service.
func NewPostgres(pool *pgxpool.Pool, stream string) *Postgres {
	h := fnv.New64a()
	_, _ = h.Write([]byte("outbox:" + stream)) // a hash never fails to write

	return &Postgres{
		pool:    pool,
		stream:  stream,
		lockKey: int64(h.Sum64()), //nolint:gosec // any 64 bits make a lock key
	}
}

const addQuery = `
	WITH added AS (
		INSERT INTO outbox (
			stream,
			kind,
			payload,
			request_id
		)
		VALUES ($1, $2, $3, $4)
		RETURNING stream
	)
	SELECT pg_notify('` + channel + `', stream)
	FROM added
`

// Add records a message in the transaction of ctx (pgpool.TxManager), with
// the request ID of ctx for the logs of its delivery.
func (q *Postgres) Add(ctx context.Context, kind string, payload []byte) error {
	conn := pgpool.Conn(ctx, q.pool)

	if _, err := conn.Exec(ctx, addQuery, q.stream, kind, payload, reqid.FromContext(ctx)); err != nil {
		return fmt.Errorf("conn.Exec: %w", err)
	}

	return nil
}

const headQuery = `
	SELECT
		id,
		kind,
		payload,
		request_id,
		attempts,
		due_at,
		created_at
	FROM outbox
	WHERE stream = $1
	ORDER BY id
	LIMIT $2
`

func (q *Postgres) Head(ctx context.Context, limit int) ([]Message, error) {
	if !q.leader {
		return nil, nil
	}

	rows, err := q.pool.Query(ctx, headQuery, q.stream, limit)
	if err != nil {
		return nil, fmt.Errorf("q.pool.Query: %w", err)
	}

	messages, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Message, error) {
		var m Message
		err := row.Scan(&m.ID, &m.Kind, &m.Payload, &m.RequestID, &m.Attempts, &m.DueAt, &m.CreatedAt)
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("pgx.CollectRows: %w", err)
	}

	return messages, nil
}

const deleteQuery = `
	DELETE FROM outbox
	WHERE id = $1
`

func (q *Postgres) Delete(ctx context.Context, id int64) error {
	if _, err := q.pool.Exec(ctx, deleteQuery, id); err != nil {
		return fmt.Errorf("q.pool.Exec: %w", err)
	}

	return nil
}

const postponeQuery = `
	UPDATE outbox
	SET attempts = $2,
		due_at = $3
	WHERE id = $1
`

func (q *Postgres) Postpone(ctx context.Context, id int64, attempts int, dueAt time.Time) error {
	if _, err := q.pool.Exec(ctx, postponeQuery, id, attempts, dueAt); err != nil {
		return fmt.Errorf("q.pool.Exec: %w", err)
	}

	return nil
}

// Wait first takes the lead: a relay that gets it returns at once to
// deliver what is waiting, one that does not tries again after d.
func (q *Postgres) Wait(ctx context.Context, d time.Duration) error {
	if !q.leader {
		return q.lead(ctx, d)
	}

	wctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()

	for {
		n, err := q.conn.WaitForNotification(wctx)

		switch {
		case err == nil:
			// The channel is shared by the streams of the table.
			if n.Payload == q.stream {
				return nil
			}
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(wctx.Err(), context.DeadlineExceeded):
			return nil // nothing new: look at postponed messages
		default:
			q.Close()
			return fmt.Errorf("q.conn.WaitForNotification: %w", err)
		}
	}
}

const lockQuery = `SELECT pg_try_advisory_lock($1)`

func (q *Postgres) lead(ctx context.Context, d time.Duration) error {
	if q.conn == nil {
		conn, err := pgx.ConnectConfig(ctx, q.pool.Config().ConnConfig.Copy())
		if err != nil {
			return fmt.Errorf("pgx.ConnectConfig: %w", err)
		}
		q.conn = conn
	}

	var locked bool
	if err := q.conn.QueryRow(ctx, lockQuery, q.lockKey).Scan(&locked); err != nil {
		q.Close()
		return fmt.Errorf("q.conn.QueryRow: %w", err)
	}

	if !locked {
		sleep(ctx, d)
		return nil
	}

	// Listen only as the leader: notifications would pile up unread on
	// the connection of a relay that waits for the lock.
	if _, err := q.conn.Exec(ctx, "LISTEN "+channel); err != nil {
		q.Close()
		return fmt.Errorf("q.conn.Exec: %w", err)
	}

	q.leader = true

	return nil
}

// Close gives up the lead, if held: the lock goes with the connection.
func (q *Postgres) Close() {
	if q.conn == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()

	_ = q.conn.Close(ctx) // the lock ends with the session either way
	q.conn = nil
	q.leader = false
}
