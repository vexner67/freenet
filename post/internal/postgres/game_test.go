package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestGameRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	documents := postgres.NewDocumentRepository(pool)
	games := postgres.NewGameRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()

	game, err := post.NewDocument(uuid.Must(uuid.NewV7()), ann, "game.html", post.DocumentHTML, 100, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = documents.Save(ctx, game); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err = games.Data(ctx, game.ID(), ann); !errors.Is(err, post.ErrGameSaveNotFound) {
		t.Fatalf("Data before a save: %v", err)
	}
	for _, data := range []post.GameData{`{"level":1}`, `{"level":2}`} {
		if err = games.SaveData(ctx, game.ID(), ann, data, now); err != nil {
			t.Fatalf("SaveData: %v", err)
		}
	}
	if data, err := games.Data(ctx, game.ID(), ann); err != nil || data != `{"level":2}` {
		t.Fatalf("Data = %q, %v", data, err)
	}

	// The best of each stays.
	for _, s := range []struct {
		who   uuid.UUID
		score post.GameScore
		best  post.GameScore
	}{{ann, 50, 50}, {ann, 30, 50}, {ann, 70, 70}, {bob, 90, 90}, {cat, 70, 70}} {
		best, err := games.RecordScore(ctx, game.ID(), s.who, s.score, now)
		if err != nil || best != s.best {
			t.Fatalf("RecordScore(%d) = %d, %v; want %d", s.score, best, err, s.best)
		}
		now = now.Add(time.Second)
	}

	records, err := games.Records(ctx, game.ID(), []uuid.UUID{ann, bob, cat, uuid.New()}, 10)
	if err != nil || len(records) != 3 || records[0].UserID != bob || records[1].UserID != ann || records[2].UserID != cat {
		t.Fatalf("Records = %+v, %v", records, err)
	}
	if records, err = games.Records(ctx, game.ID(), []uuid.UUID{ann, cat}, 1); err != nil || len(records) != 1 || records[0].Score != 70 || records[0].UserID != ann {
		t.Fatalf("Records of two, one = %+v, %v", records, err)
	}

	if err = games.DeleteByUser(ctx, ann); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if _, err = games.Data(ctx, game.ID(), ann); !errors.Is(err, post.ErrGameSaveNotFound) {
		t.Fatalf("Data after the purge: %v", err)
	}
	if records, _ = games.Records(ctx, game.ID(), []uuid.UUID{ann, bob}, 10); len(records) != 1 || records[0].UserID != bob {
		t.Fatalf("Records after the purge = %+v", records)
	}

	// A deleted widget takes its saves and scores with it.
	if err = games.SaveData(ctx, game.ID(), bob, "{}", now); err != nil {
		t.Fatal(err)
	}
	if err = documents.Delete(ctx, game.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = games.Data(ctx, game.ID(), bob); !errors.Is(err, post.ErrGameSaveNotFound) {
		t.Fatalf("Data of a deleted game: %v", err)
	}
	if records, _ = games.Records(ctx, game.ID(), []uuid.UUID{bob}, 10); len(records) != 0 {
		t.Fatalf("Records of a deleted game = %+v", records)
	}
}
