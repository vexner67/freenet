package model_test

import (
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
)

func TestFirstLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text, want string
	}{
		{"нет ссылок", ""},
		{"Смотрите: https://example.com/a?b=1#c", "https://example.com/a?b=1#c"},
		{"http://example.com, и ещё https://other.org", "http://example.com"},
		{"Вот (https://ru.wikipedia.org/wiki/Кот_(значения)).", "https://ru.wikipedia.org/wiki/Кот_(значения)"},
		{"«https://example.com»", "https://example.com"},
		{"https://example.com/путь!", "https://example.com/путь"},
		{"ftp://example.com", ""},
		{"xhttps://example.com", ""},
		{"https://", ""},
		{"[ann|https://example.com]", "https://example.com"},
	}

	for _, tt := range tests {
		if got := model.FirstLink(tt.text); got != tt.want {
			t.Errorf("FirstLink(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}
