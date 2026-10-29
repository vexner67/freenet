package user_test

import (
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestMulDiv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		a, b, d int64
		want    int64
		up      int64
	}{
		{name: "small", a: 7, b: 150, d: 100, want: 10, up: 11},
		{name: "exact", a: 4, b: 25, d: 100, want: 1, up: 1},
		{name: "zero", a: 0, b: user.MaxMultiplier, d: 100, want: 0, up: 0},
		{name: "past int64 and back", a: user.MaxMoved, b: user.MaxMultiplier, d: user.MaxMultiplier, want: user.MaxMoved, up: user.MaxMoved},
		{name: "past the most a win pays", a: user.MaxMoved, b: user.MaxMultiplier, d: 100, want: user.MaxPayout, up: user.MaxPayout},
		{name: "toward zero below it", a: -7, b: 150, d: 100, want: -10},
		{name: "below the least", a: -user.MaxMoved, b: user.MaxMultiplier, d: 1, want: -user.MaxPayout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := user.MulDiv(tt.a, tt.b, tt.d); got != tt.want {
				t.Fatalf("MulDiv(%d, %d, %d) = %d, want %d", tt.a, tt.b, tt.d, got, tt.want)
			}
			if tt.a < 0 {
				return
			}
			if got := user.MulDivUp(tt.a, tt.b, tt.d); got != tt.up {
				t.Fatalf("MulDivUp(%d, %d, %d) = %d, want %d", tt.a, tt.b, tt.d, got, tt.up)
			}
		})
	}
}
