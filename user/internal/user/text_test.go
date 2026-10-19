package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestInvisibleText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
		err   error
	}{
		{"plain", "Анна", "Анна", nil},
		{"spaces around", "  Анна\u00a0\u3000", "Анна", nil},
		{"emoji with joiners", "Анна 👩\u200d💻", "Анна 👩\u200d💻", nil},
		{"emoji with a variation selector", "Анна ❤\ufe0f", "Анна ❤\ufe0f", nil},
		{"joiner at the end", "Анна\u200d", "Анна", nil},
		{"Hangul filler", "\u3164", "", user.ErrInvisibleText},
		{"halfwidth Hangul filler", "Анна\uffa0", "", user.ErrInvisibleText},
		{"Braille blank", "\u2800\u2800", "", user.ErrInvisibleText},
		{"zero-width space inside", "Ан\u200bна", "", user.ErrInvisibleText},
		{"word joiner", "\u2060", "", user.ErrInvisibleText},
		{"byte order mark", "\ufeffАнна", "", user.ErrInvisibleText},
		{"soft hyphen", "Ан\u00adна", "", user.ErrInvisibleText},
		{"bell", "Анна\a", "", user.ErrInvisibleText},
		{"right-to-left override", "\u202eАнна", "", user.ErrInvisibleText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, err := user.NewStatus(tt.input)
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewStatus(%q) error = %v, want %v", tt.input, err, tt.err)
			}

			if status.String() != tt.want {
				t.Errorf("NewStatus(%q) = %q, want %q", tt.input, status, tt.want)
			}
		})
	}
}

// Every text of the page refuses what shows nothing, and a text of
// joiners only is empty.
func TestInvisibleText_EveryField(t *testing.T) {
	t.Parallel()

	const filler = "\u3164"

	checks := map[string]func(string) error{
		"name":          func(s string) error { _, err := user.NewName(s); return err },
		"surname":       func(s string) error { _, err := user.NewSurname(s); return err },
		"status":        func(s string) error { _, err := user.NewStatus(s); return err },
		"place":         func(s string) error { _, err := user.NewPlace(s); return err },
		"address":       func(s string) error { _, err := user.NewAddress(s); return err },
		"website":       func(s string) error { _, err := user.NewWebsite(s); return err },
		"contact email": func(s string) error { _, err := user.NewContactEmail(s); return err },
		"about":         func(s string) error { _, err := user.NewAbout(s); return err },
		"field name": func(s string) error {
			_, err := user.NewExtraField(s, "текст", user.FieldPlaceContacts)
			return err
		},
	}

	for name, check := range checks {
		if err := check(filler); !errors.Is(err, user.ErrInvisibleText) {
			t.Errorf("%s of a filler: error = %v, want %v", name, err, user.ErrInvisibleText)
		}
	}

	if _, err := user.NewName("\u200d "); !errors.Is(err, user.ErrNameEmpty) {
		t.Errorf("name of a joiner: error = %v, want %v", err, user.ErrNameEmpty)
	}

	if _, err := user.NewExtraField("Поле", "\u200d", user.FieldPlaceContacts); !errors.Is(err, user.ErrExtraFieldTextEmpty) {
		t.Errorf("field text of a joiner: error = %v, want %v", err, user.ErrExtraFieldTextEmpty)
	}
}
