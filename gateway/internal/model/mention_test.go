package model_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
)

func TestMentionedNicknames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want []string
	}{
		{"none", "просто текст, mail@example.com и [не ссылка]", nil},
		{"at the start", "@ann привет", []string{"ann"}},
		{"after a space and a line break", "привет, @ann!\n@bob_1, смотри", []string{"ann", "bob_1"}},
		{"in a word is not a mention", "почта:ann@bob", nil},
		{"a trailing underscore is not of the nickname", "@ann_ и @bob__.", []string{"ann", "bob"}},
		{"a reply", "[cat|Катя Сидорова], согласна", []string{"cat"}},
		{"both ways, each once whatever the case", "[Ann|Аня], @ann и @ANN, @bob", []string{"Ann", "bob"}},
		{"not a nickname", "@1ann @_ann [a b|Имя] [ann_|Аня] [ann|] @я", nil},
		{"too long", "@" + strings.Repeat("a", 33), nil},
		{"the longest", "@" + strings.Repeat("a", 32), []string{strings.Repeat("a", 32)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := model.MentionedNicknames(tt.text); !slices.Equal(got, tt.want) {
				t.Fatalf("MentionedNicknames(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}

	var many []string
	for _, c := range "abcdefghijkl" {
		many = append(many, "@user"+string(c))
	}
	if got := model.MentionedNicknames(strings.Join(many, " ")); len(got) != model.MaxMentions || got[9] != "userj" {
		t.Fatalf("the first ten of twelve = %v", got)
	}
}
