package reqid_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/vexner67/freenet/pkg/reqid"
)

func TestValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "generated", id: reqid.New(), want: true},
		{name: "uuid", id: "0199b2a4-6f1e-7c3a-9d2b-3e4f5a6b7c8d", want: true},
		{name: "proxy style", id: "edge-1:42.7_a", want: true},
		{name: "empty", id: "", want: false},
		{name: "longest", id: strings.Repeat("a", 64), want: true},
		{name: "too long", id: strings.Repeat("a", 65), want: false},
		{name: "newline", id: "abc\nlevel=ERROR msg=forged", want: false},
		{name: "space", id: "a b", want: false},
		{name: "quote", id: `a"b`, want: false},
		{name: "cyrillic", id: "запрос", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := reqid.Valid(tt.id); got != tt.want {
				t.Errorf("Valid(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestNew_Unique(t *testing.T) {
	t.Parallel()

	if a, b := reqid.New(), reqid.New(); a == b {
		t.Errorf("New() returned %q twice", a)
	}
}

func TestLogHandler(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log := slog.New(reqid.LogHandler(slog.NewTextHandler(&buf, nil))).With("service", "test")

	log.InfoContext(reqid.ContextWithID(context.Background(), "abc"), "with id")
	log.InfoContext(context.Background(), "without id")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), buf.String())
	}

	if !strings.Contains(lines[0], "request_id=abc") || !strings.Contains(lines[0], "service=test") {
		t.Errorf("line with an ID = %q, want request_id=abc and service=test", lines[0])
	}

	if strings.Contains(lines[1], "request_id") {
		t.Errorf("line without an ID = %q, want no request_id", lines[1])
	}
}
