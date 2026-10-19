package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewVoteRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kopecks int64
		err     error
	}{
		{name: "a kopeck", kopecks: 1},
		{name: "seven rubles", kopecks: 700},
		{name: "a million rubles", kopecks: user.MaxVoteRate},
		{name: "free", kopecks: 0, err: user.ErrVoteRateInvalid},
		{name: "negative", kopecks: -700, err: user.ErrVoteRateInvalid},
		{name: "too dear", kopecks: user.MaxVoteRate + 1, err: user.ErrVoteRateInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rate, err := user.NewVoteRate(tt.kopecks)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v; want %v", err, tt.err)
			}
			if err == nil && rate.Kopecks() != tt.kopecks {
				t.Fatalf("kopecks = %d; want %d", rate.Kopecks(), tt.kopecks)
			}
		})
	}
}
