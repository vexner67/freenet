package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestCanvasService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann := w.person(t, "ann")
	canvas := service.NewCanvasService(inmem.NewCanvasRepository(w.users))
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

	picture, err := canvas.Canvas(ctx)
	if err != nil || len(picture) != user.CanvasSize*user.CanvasSize {
		t.Fatalf("an empty canvas = %d, %v", len(picture), err)
	}
	if _, err = canvas.Cell(ctx, 5, 5); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("a cell never painted: %v", err)
	}
	if _, err = canvas.Cell(ctx, -1, 5); !errors.Is(err, user.ErrCanvasCell) {
		t.Fatalf("a cell off the canvas: %v", err)
	}

	cell, err := user.NewCanvasCell(5, 6, 3, ann, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = canvas.Paint(ctx, cell); err != nil {
		t.Fatal(err)
	}

	// There is no pause: the next cell is painted at once.
	cell.X = 7
	if err = canvas.Paint(ctx, cell); err != nil {
		t.Fatalf("the next cell: %v", err)
	}
	if picture, err = canvas.Canvas(ctx); err != nil || picture[6*user.CanvasSize+5] != 3 || picture[6*user.CanvasSize+7] != 3 {
		t.Fatalf("the canvas after two cells: %v", err)
	}
	if got, err := canvas.Cell(ctx, 7, 6); err != nil || got.UserID != ann || got.Color != 3 {
		t.Fatalf("Cell = %+v, %v", got, err)
	}

	// Nobody paints for a page that is not there.
	cell.UserID = uuid.New()
	if err = canvas.Paint(ctx, cell); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Paint by nobody: %v", err)
	}
}
