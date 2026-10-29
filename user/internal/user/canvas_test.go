package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestCanvas(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		x, y, color int
		err         error
	}{
		{"the corner", 0, 0, 0, nil},
		{"the far corner", user.CanvasSize - 1, user.CanvasSize - 1, user.CanvasColors - 1, nil},
		{"left of it", -1, 0, 1, user.ErrCanvasCell},
		{"below it", 0, user.CanvasSize, 1, user.ErrCanvasCell},
		{"no such color", 1, 1, user.CanvasColors, user.ErrCanvasColor},
		{"a negative color", 1, 1, -1, user.ErrCanvasColor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := user.NewCanvasCell(tt.x, tt.y, tt.color, uuid.New(), now); !errors.Is(err, tt.err) {
				t.Fatalf("NewCanvasCell = %v, want %v", err, tt.err)
			}
		})
	}

	c, err := user.NewCanvasCell(3, 2, 7, uuid.New(), now)
	if err != nil {
		t.Fatal(err)
	}
	picture := user.CanvasPicture([]user.CanvasCell{c})
	if len(picture) != user.CanvasSize*user.CanvasSize || picture[2*user.CanvasSize+3] != 7 || picture[0] != 0 {
		t.Fatalf("CanvasPicture = %v", picture[:5])
	}

	if !user.CanvasNextAt(time.Time{}).IsZero() || !user.CanvasNextAt(now).Equal(now.Add(user.CanvasCooldown)) {
		t.Fatal("CanvasNextAt")
	}
}
