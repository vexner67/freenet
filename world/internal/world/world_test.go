package world_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/world/internal/world"
)

var t0 = time.Date(2026, 10, 28, 10, 30, 0, 0, time.UTC)

func TestWorldLife(t *testing.T) {
	t.Parallel()

	if _, err := world.New(1, 1000, t0); !errors.Is(err, world.ErrGrid) {
		t.Fatalf("New on a bad grid: %v", err)
	}

	w, err := world.New(1, 2048, t0)
	if err != nil || w.Status() != world.StatusGenerating {
		t.Fatalf("New = %v, %v", w, err)
	}
	if err = w.Open(t0); !errors.Is(err, world.ErrNotReady) {
		t.Fatalf("Open before the map: %v", err)
	}
	if err = w.End(t0); !errors.Is(err, world.ErrNotOpen) {
		t.Fatalf("End before it opened: %v", err)
	}

	w.Made()
	if err = w.Open(t0); err != nil || w.Status() != world.StatusOpen || !w.OpenedAt().Equal(t0) {
		t.Fatalf("Open = %v, %v", w.Status(), err)
	}
	if err = w.End(t0); err != nil || w.Status() != world.StatusEnded {
		t.Fatalf("End = %v, %v", w.Status(), err)
	}

	f, _ := world.New(2, 1024, t0)
	f.Failed()
	f.Made()
	if f.Status() != world.StatusFailed {
		t.Fatalf("a failed world became %s", f.Status())
	}
}
