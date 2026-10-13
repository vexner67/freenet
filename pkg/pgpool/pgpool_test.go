package pgpool_test

import (
	"context"
	"net/url"
	"os"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/pgpool"
)

const (
	createThings = "-- +goose Up\nCREATE TABLE things (id int PRIMARY KEY);\n-- +goose Down\nDROP TABLE things;\n"
	nameThings   = "-- +goose Up\nALTER TABLE things ADD COLUMN name text;\n-- +goose Down\nALTER TABLE things DROP COLUMN name;\n"
	fillThings   = "-- +goose Up\nINSERT INTO things (id, name) VALUES (1, 'one');\n-- +goose Down\nDELETE FROM things;\n"
)

// newSchema makes a schema of its own in TEST_DATABASE_URL and returns a
// DSN whose connections work in it, with a pool on that DSN: goose keeps
// its version table in the schema too, so the test neither sees nor leaves
// anything in the database the other packages share.
func newSchema(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()

	admin := newPool(t)
	ctx := context.Background()

	const schema = "pgpool_migrate_test"
	if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE; CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })

	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	pool, err := pgpool.New(ctx, u.String())
	if err != nil {
		t.Fatalf("pgpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	return u.String(), pool
}

func TestMigrate(t *testing.T) {
	dsn, pool := newSchema(t)
	ctx := context.Background()

	migrations := fstest.MapFS{
		"00001_create_things.sql": {Data: []byte(createThings)},
		"00002_name_things.sql":   {Data: []byte(nameThings)},
	}

	applied, err := pgpool.Migrate(ctx, dsn, migrations)
	if err != nil || applied != 2 {
		t.Fatalf("first run applied %d, %v; want 2", applied, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO things (id, name) VALUES (7, 'seven')`); err != nil {
		t.Fatalf("the migrated table: %v", err)
	}

	// Nothing is pending: a restart changes nothing.
	if applied, err = pgpool.Migrate(ctx, dsn, migrations); err != nil || applied != 0 {
		t.Fatalf("second run applied %d, %v; want 0", applied, err)
	}

	// A new version brings one more migration and applies only it.
	migrations["00003_fill_things.sql"] = &fstest.MapFile{Data: []byte(fillThings)}
	if applied, err = pgpool.Migrate(ctx, dsn, migrations); err != nil || applied != 1 {
		t.Fatalf("third run applied %d, %v; want 1", applied, err)
	}

	var things int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM things`).Scan(&things); err != nil || things != 2 {
		t.Fatalf("things = %d, %v; want 2", things, err)
	}
}

// Replicas starting at once migrate under the advisory lock: one applies
// the migrations, the others wait and find nothing to do.
func TestMigrate_Concurrent(t *testing.T) {
	dsn, pool := newSchema(t)
	ctx := context.Background()

	migrations := fstest.MapFS{
		"00001_create_things.sql": {Data: []byte(createThings)},
		"00002_name_things.sql":   {Data: []byte(nameThings)},
		"00003_fill_things.sql":   {Data: []byte(fillThings)},
	}

	const replicas = 5

	var (
		wg      sync.WaitGroup
		applied [replicas]int
		failed  [replicas]error
	)
	for i := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			applied[i], failed[i] = pgpool.Migrate(ctx, dsn, migrations)
		}()
	}
	wg.Wait()

	total := 0
	for i := range replicas {
		if failed[i] != nil {
			t.Fatalf("replica %d: %v", i, failed[i])
		}
		total += applied[i]
	}
	if total != len(migrations) {
		t.Fatalf("replicas applied %d migrations in all, want %d", total, len(migrations))
	}

	var things int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM things`).Scan(&things); err != nil || things != 1 {
		t.Fatalf("things = %d, %v; want 1 (the filling migration ran once)", things, err)
	}
}

func TestMigrate_Broken(t *testing.T) {
	dsn, pool := newSchema(t)
	ctx := context.Background()

	migrations := fstest.MapFS{
		"00001_create_things.sql": {Data: []byte(createThings)},
		"00002_break.sql":         {Data: []byte("-- +goose Up\nALTER TABLE nothing ADD COLUMN name text;\n")},
	}

	if _, err := pgpool.Migrate(ctx, dsn, migrations); err == nil {
		t.Fatal("a failing migration: no error")
	}

	// What was applied before the failure stays, and the lock is released:
	// the fixed version migrates on.
	migrations["00002_break.sql"] = &fstest.MapFile{Data: []byte(nameThings)}
	applied, err := pgpool.Migrate(ctx, dsn, migrations)
	if err != nil || applied != 1 {
		t.Fatalf("the fixed run applied %d, %v; want 1", applied, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO things (id, name) VALUES (1, 'one')`); err != nil {
		t.Fatalf("the migrated table: %v", err)
	}
}

func TestMigrate_Unreachable(t *testing.T) {
	_, err := pgpool.Migrate(context.Background(), "postgres://nobody@127.0.0.1:1/nothing?connect_timeout=1", fstest.MapFS{
		"00001_create_things.sql": {Data: []byte(createThings)},
	})
	if err == nil {
		t.Fatal("an unreachable database: no error")
	}
}
