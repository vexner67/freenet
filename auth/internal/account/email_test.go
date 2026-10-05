package account_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/vexner67/freenet/auth/internal/account"
)

func TestNewEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  account.Email
		err   error
	}{
		{"plain", "ann@example.com", "ann@example.com", nil},
		{"canonical case and spaces", "  Ann@Example.COM ", "ann@example.com", nil},
		{"empty", "", "", account.ErrInvalidEmail},
		{"spaces only", "   ", "", account.ErrInvalidEmail},
		{"no domain", "ann@", "", account.ErrInvalidEmail},
		{"display name", "Ann <ann@example.com>", "", account.ErrInvalidEmail},
		{"too long", strings.Repeat("a", 250) + "@x.io", "", account.ErrInvalidEmail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := account.NewEmail(tt.input)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("NewEmail(%q) = %q, %v; want %q, %v", tt.input, got, err, tt.want, tt.err)
			}
		})
	}
}
