package document_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/media/internal/document"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	if err := document.Check(nil); !errors.Is(err, document.ErrEmpty) {
		t.Fatalf("empty: %v", err)
	}
	if err := document.Check(make([]byte, document.MaxBytes)); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	if err := document.Check(make([]byte, document.MaxBytes+1)); !errors.Is(err, document.ErrTooBig) {
		t.Fatalf("over the limit: %v", err)
	}
}
