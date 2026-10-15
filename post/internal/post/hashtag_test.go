package post_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vexner67/freenet/post/internal/post"
)

func TestText_Hashtags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want []post.Hashtag
	}{
		{name: "none", text: "в поход", want: nil},
		{name: "start", text: "#поход завтра", want: []post.Hashtag{"поход"}},
		{name: "after a space", text: "идём\n#Лес и #summer_2026", want: []post.Hashtag{"лес", "summer_2026"}},
		{name: "ends at a mark", text: "#поход, #лес!", want: []post.Hashtag{"поход", "лес"}},
		{name: "inside a word", text: "C# и url/#anchor", want: nil},
		{name: "glued tags", text: "#поход#лес", want: []post.Hashtag{"поход"}},
		{name: "once", text: "#Поход #поход #ПОХОД", want: []post.Hashtag{"поход"}},
		{name: "bare sign", text: "# и #", want: nil},
		{name: "digits", text: "#2026", want: []post.Hashtag{"2026"}},
		{name: "too long", text: "#" + strings.Repeat("я", 65) + " #ok", want: []post.Hashtag{"ok"}},
		{name: "longest", text: "#" + strings.Repeat("я", 64), want: []post.Hashtag{post.Hashtag(strings.Repeat("я", 64))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := post.Text(tt.text).Hashtags(); !slices.Equal(got, tt.want) {
				t.Errorf("Hashtags() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestText_Hashtags_Limit(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for i := range post.MaxHashtags + 5 {
		b.WriteString("#t")
		b.WriteString(strings.Repeat("x", i+1))
		b.WriteString(" ")
	}

	if got := post.Text(b.String()).Hashtags(); len(got) != post.MaxHashtags || got[0] != "tx" {
		t.Errorf("Hashtags() = %d tags from %q, want %d from tx", len(got), got[0], post.MaxHashtags)
	}
}

func TestNewSearchQuery(t *testing.T) {
	t.Parallel()

	q, err := post.NewSearchQuery("  #Поход  ")
	if err != nil || q != "#Поход" {
		t.Fatalf("NewSearchQuery() = %q, %v", q, err)
	}

	if _, err := post.NewSearchQuery(strings.Repeat("я", 101)); !errors.Is(err, post.ErrSearchQueryTooLong) {
		t.Errorf("NewSearchQuery(101 runes) error = %v, want ErrSearchQueryTooLong", err)
	}
}

func TestSearchQuery_Hashtag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  post.Hashtag
		ok    bool
	}{
		{query: "#Поход", want: "поход", ok: true},
		{query: "#поход лес", ok: false},
		{query: "#поход!", ok: false},
		{query: "поход", ok: false},
		{query: "#", ok: false},
		{query: "", ok: false},
	}

	for _, tt := range tests {
		got, ok := post.SearchQuery(tt.query).Hashtag()
		if got != tt.want || ok != tt.ok {
			t.Errorf("SearchQuery(%q).Hashtag() = %q, %v, want %q, %v", tt.query, got, ok, tt.want, tt.ok)
		}
	}
}

func TestSearchQuery_Matches(t *testing.T) {
	t.Parallel()

	text, err := post.NewText("Идём в #Походы на выходных")
	if err != nil {
		t.Fatal(err)
	}

	content, err := post.NewContent(text, nil)
	if err != nil {
		t.Fatal(err)
	}

	id := uuid.New()

	p, err := post.New(id, id, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		query string
		want  bool
	}{
		{query: "#походы", want: true},
		{query: "#поход", want: false},
		{query: "в #поход", want: true},
		{query: "ВЫХОДНЫХ", want: true},
		{query: "горы", want: false},
	}

	for _, tt := range tests {
		if got := post.SearchQuery(tt.query).Matches(p); got != tt.want {
			t.Errorf("SearchQuery(%q).Matches() = %v, want %v", tt.query, got, tt.want)
		}
	}
}
