package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

func newGames(t *testing.T) (*service.GameService, uuid.UUID, uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	documents := inmem.NewDocumentRepository()
	owner := uuid.New()

	add := func(name post.DocumentName, ext post.DocumentExt) uuid.UUID {
		d, err := post.NewDocument(uuid.Must(uuid.NewV7()), owner, name, ext, 100, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = documents.Save(ctx, d); err != nil {
			t.Fatal(err)
		}
		return d.ID()
	}

	return service.NewGameService(inmem.NewGameRepository(), documents), add("game.html", post.DocumentHTML), add("book.pdf", "pdf")
}

func TestGameService_Saves(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	games, game, book := newGames(t)
	ann, bob := uuid.New(), uuid.New()

	if _, found, err := games.Load(ctx, ann, game); err != nil || found {
		t.Fatalf("Load before a save = %v, %v", found, err)
	}
	if err := games.Save(ctx, ann, game, `{"level":4}`); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if data, found, err := games.Load(ctx, ann, game); err != nil || !found || data != `{"level":4}` {
		t.Fatalf("Load = %q, %v, %v", data, found, err)
	}
	// Each player has a save of their own.
	if _, found, err := games.Load(ctx, bob, game); err != nil || found {
		t.Fatalf("Load of another = %v, %v", found, err)
	}

	// Only an HTML widget keeps saves.
	if err := games.Save(ctx, ann, book, "{}"); !errors.Is(err, post.ErrGameNotFound) {
		t.Fatalf("Save in a pdf: %v", err)
	}
	if _, _, err := games.Load(ctx, ann, uuid.New()); !errors.Is(err, post.ErrDocumentNotFound) {
		t.Fatalf("Load of a missing game: %v", err)
	}
}

func TestGameService_Records(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	games, game, book := newGames(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	for _, s := range []struct {
		who         uuid.UUID
		score, best post.GameScore
	}{{ann, 10, 10}, {ann, 5, 10}, {bob, 30, 30}, {cat, 20, 20}} {
		best, err := games.Score(ctx, s.who, game, s.score)
		if err != nil || best != s.best {
			t.Fatalf("Score(%d) = %d, %v; want %d", s.score, best, err, s.best)
		}
	}

	records, err := games.Records(ctx, game, []uuid.UUID{ann, bob}, 0)
	if err != nil || len(records) != 2 || records[0].UserID != bob || records[1].Score != 10 {
		t.Fatalf("Records = %+v, %v", records, err)
	}
	if records, err = games.Records(ctx, game, nil, 10); err != nil || len(records) != 0 {
		t.Fatalf("Records of nobody = %+v, %v", records, err)
	}
	if _, err = games.Records(ctx, game, make([]uuid.UUID, service.MaxRecordPlayers+1), 10); !errors.Is(err, service.ErrTooManyIDs) {
		t.Fatalf("Records of too many: %v", err)
	}
	if _, err = games.Score(ctx, ann, book, 1); !errors.Is(err, post.ErrGameNotFound) {
		t.Fatalf("Score in a pdf: %v", err)
	}
	if _, err = games.Records(ctx, book, []uuid.UUID{ann}, 10); !errors.Is(err, post.ErrGameNotFound) {
		t.Fatalf("Records of a pdf: %v", err)
	}
}
