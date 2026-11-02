package apjson_test

import (
	"strings"
	"testing"

	"github.com/vexner67/freenet/federation/internal/apjson"
	"github.com/vexner67/freenet/federation/internal/fedi"
)

func TestPlainText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{"paragraphs", "<p>one</p><p>two<br>three</p>", "one\n\ntwo\nthree"},
		{"entities", "<p>a &amp; b &lt;3</p>", "a & b <3"},
		{
			"link written whole",
			`<p>see <a href="https://example.com/a/very/long/path" rel="nofollow"><span class="invisible">https://</span><span class="ellipsis">example.com/a/very</span></a></p>`,
			"see https://example.com/a/very/long/path",
		},
		{
			"mention and hashtag as words",
			`<p><span class="h-card"><a href="https://m.example/@ann" class="u-url mention">@<span>ann</span></a></span> <a href="https://m.example/tags/go" class="mention hashtag">#<span>go</span></a></p>`,
			"@ann #go",
		},
		{"no scripts", "<p>hi</p><script>alert(1)</script><style>p{}</style>", "hi"},
		{"plain", "just text", "just text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := apjson.PlainText(tt.in); got != tt.want {
				t.Errorf("PlainText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIncomingNote(t *testing.T) {
	t.Parallel()

	a, err := apjson.Incoming([]byte(`{"id":"https://m.example/notes/1/activity","type":"Create","actor":"https://m.example/users/ann",
		"object":{"id":"https://m.example/notes/1","type":"Note","attributedTo":"https://m.example/users/ann",
			"url":"https://m.example/@ann/1","inReplyTo":null,"content":"<p>hello</p>","contentMap":{"en":"<p>hello</p>"},
			"summary":"cw","sensitive":true,"to":["https://www.w3.org/ns/activitystreams#Public"],"cc":[],
			"published":"2026-11-01T10:00:00Z",
			"attachment":[{"type":"Document","mediaType":"image/png","url":"https://m.example/1.png","name":"a cat"}]}}`))
	if err != nil {
		t.Fatalf("Incoming: %v", err)
	}

	n := a.Note
	if n == nil || n.ID != "https://m.example/notes/1" || n.AttributedTo != "https://m.example/users/ann" || n.URL != "https://m.example/@ann/1" ||
		n.Text != "hello" || n.Summary != "cw" || !n.Sensitive || !n.Public || n.Language != "en" || n.InReplyTo != "" ||
		n.Published.IsZero() || len(n.Attachments) != 1 || n.Attachments[0].Description != "a cat" {
		t.Fatalf("note = %+v", n)
	}

	if a.Object != "https://m.example/notes/1" {
		t.Errorf("object = %q, want the ID of the note", a.Object)
	}

	followers, err := apjson.Incoming([]byte(`{"type":"Create","actor":"https://m.example/users/ann",
		"object":{"id":"https://m.example/notes/2","type":"Note","content":"x","to":"https://m.example/users/ann/followers"}}`))
	if err != nil || followers.Note == nil || followers.Note.Public {
		t.Fatalf("a note for followers = %+v, %v", followers.Note, err)
	}

	question, err := apjson.Incoming([]byte(`{"type":"Create","actor":"https://m.example/users/ann",
		"object":{"id":"https://m.example/q/1","type":"Question"}}`))
	if err != nil || question.Note != nil {
		t.Fatalf("a question = %+v, %v; want no note", question.Note, err)
	}
}

func TestPersonOfSite(t *testing.T) {
	t.Parallel()

	doc, err := apjson.Person(fedi.Person{ID: "https://f.example/ap/actor", Service: true})
	if err != nil {
		t.Fatalf("Person: %v", err)
	}

	if !strings.Contains(string(doc), `"type":"Application"`) {
		t.Fatalf("the site is %s, want an Application", doc)
	}
}
