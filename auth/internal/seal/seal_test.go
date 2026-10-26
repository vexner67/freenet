package seal_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/vexner67/freenet/auth/internal/seal"
)

func TestBox(t *testing.T) {
	t.Parallel()

	box, err := seal.New("hash-secret", "two-factor")
	if err != nil {
		t.Fatal(err)
	}

	plain := []byte("12345678901234567890")
	sealed, err := box.Seal(plain, "ann")
	if err != nil || bytes.Contains(sealed, plain) {
		t.Fatalf("Seal = %x, %v", sealed, err)
	}

	again, _ := box.Seal(plain, "ann")
	if bytes.Equal(sealed, again) {
		t.Fatal("two seals of the same text are equal")
	}

	if got, err := box.Open(sealed, "ann"); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("Open = %q, %v", got, err)
	}

	other, _ := seal.New("hash-secret", "sessions")
	damaged := append([]byte{}, sealed...)
	damaged[len(damaged)-1] ^= 1

	for name, open := range map[string]func() ([]byte, error){
		"another label":   func() ([]byte, error) { return box.Open(sealed, "bob") },
		"another purpose": func() ([]byte, error) { return other.Open(sealed, "ann") },
		"damaged":         func() ([]byte, error) { return box.Open(damaged, "ann") },
		"short":           func() ([]byte, error) { return box.Open(sealed[:4], "ann") },
	} {
		if _, err := open(); !errors.Is(err, seal.ErrSealed) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
