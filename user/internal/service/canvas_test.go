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
	canvas := service.NewCanvasService(inmem.NewCanvasRepository(w.users), inmem.TxManager{})
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

	view, err := canvas.Canvas(ctx, ann, now)
	if err != nil || len(view.Picture) != user.CanvasSize*user.CanvasSize || !view.NextAt.IsZero() {
		t.Fatalf("an empty canvas = %+v, %v", view.NextAt, err)
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
	next, err := canvas.Paint(ctx, cell)
	if err != nil || !next.Equal(now.Add(user.CanvasCooldown)) {
		t.Fatalf("Paint = %v, %v", next, err)
	}

	// Another cell within the minute waits; after it, it is painted.
	cell.X, cell.At = 7, now.Add(30*time.Second)
	if _, err = canvas.Paint(ctx, cell); !errors.Is(err, user.ErrCanvasCooldown) {
		t.Fatalf("Paint within the minute: %v", err)
	}
	if view, err = canvas.Canvas(ctx, ann, now.Add(30*time.Second)); err != nil || !view.NextAt.Equal(next) || view.Picture[6*user.CanvasSize+5] != 3 {
		t.Fatalf("the canvas after a cell = %v, %v", view.NextAt, err)
	}
	cell.At = next
	if _, err = canvas.Paint(ctx, cell); err != nil {
		t.Fatal(err)
	}
	if got, err := canvas.Cell(ctx, 7, 6); err != nil || got.UserID != ann || got.Color != 3 {
		t.Fatalf("Cell = %+v, %v", got, err)
	}

	// A guest sees the canvas and no wait; nobody paints for a page that
	// is not there.
	if view, err = canvas.Canvas(ctx, uuid.Nil, now); err != nil || !view.NextAt.IsZero() {
		t.Fatalf("the canvas of a guest = %v, %v", view.NextAt, err)
	}
	cell.UserID = uuid.New()
	if _, err = canvas.Paint(ctx, cell); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Paint by nobody: %v", err)
	}
}
