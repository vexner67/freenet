package captcha_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/vexner67/freenet/auth/internal/captcha"
)

func TestNew(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)

	for range 200 {
		answer, image, err := captcha.New()
		if err != nil {
			t.Fatal(err)
		}
		if len(answer) != captcha.Length || strings.Trim(answer, "0123456789") != "" {
			t.Fatalf("answer = %q", answer)
		}
		if strings.Contains(image, "<text") || strings.Contains(image, answer) {
			t.Fatalf("the picture gives the answer away: %s", image)
		}
		if err = xml.Unmarshal([]byte(image), new(struct{})); err != nil {
			t.Fatalf("the picture is no XML: %v", err)
		}
		seen[answer] = true
	}

	if len(seen) < 150 {
		t.Fatalf("only %d different answers of 200", len(seen))
	}
}
