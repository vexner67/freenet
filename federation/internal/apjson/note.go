package apjson

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"golang.org/x/net/html"
)

// note is a note as another server writes it. Many of its properties may
// be an ID, an object or a list of either; flexible reads them all.
type noteDoc struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	AttributedTo flexible          `json:"attributedTo"`
	URL          flexible          `json:"url"`
	InReplyTo    flexible          `json:"inReplyTo"`
	Content      string            `json:"content"`
	ContentMap   map[string]string `json:"contentMap"`
	Summary      string            `json:"summary"`
	Sensitive    bool              `json:"sensitive"`
	To           flexible          `json:"to"`
	CC           flexible          `json:"cc"`
	Published    string            `json:"published"`
	Updated      string            `json:"updated"`
	Attachment   []struct {
		MediaType string          `json:"mediaType"`
		URL       flexible        `json:"url"`
		Name      string          `json:"name"`
		Icon      json.RawMessage `json:"icon"`
		Blurhash  string          `json:"blurhash"`
		Width     int             `json:"width"`
		Height    int             `json:"height"`
		Duration  string          `json:"duration"`
	} `json:"attachment"`
}

// flexible is the IDs of a property: a string, an object with an id or
// an href, or a list of them.
type flexible []string

func (f *flexible) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*f = flexible{one}
		return nil
	}

	var many []json.RawMessage
	if json.Unmarshal(data, &many) == nil {
		for _, m := range many {
			var item flexible
			if item.UnmarshalJSON(m) == nil {
				*f = append(*f, item...)
			}
		}
		return nil
	}

	var obj struct {
		ID   string `json:"id"`
		Href string `json:"href"`
	}
	if json.Unmarshal(data, &obj) == nil {
		switch {
		case obj.ID != "":
			*f = flexible{obj.ID}
		case obj.Href != "":
			*f = flexible{obj.Href}
		}
	}

	return nil
}

func (f flexible) first() string {
	if len(f) == 0 {
		return ""
	}

	return f[0]
}

// public are the ways a note says it is for everyone.
var public = []string{fedi.Public, "as:Public", "Public"}

// toNoteDoc reads a note; nil when the object is no note.
func toNoteDoc(raw json.RawMessage) *fedi.NoteDoc {
	var n noteDoc
	if err := json.Unmarshal(raw, &n); err != nil || n.Type != "Note" {
		return nil
	}

	content, lang := n.Content, ""
	if len(n.ContentMap) == 1 {
		for l, c := range n.ContentMap {
			lang = l
			if content == "" {
				content = c
			}
		}
	}

	doc := &fedi.NoteDoc{
		ID:           n.ID,
		AttributedTo: n.AttributedTo.first(),
		URL:          n.URL.first(),
		InReplyTo:    n.InReplyTo.first(),
		Text:         PlainText(content),
		Summary:      PlainText(n.Summary),
		Language:     lang,
		Sensitive:    n.Sensitive,
		Public:       slices.ContainsFunc(append(n.To, n.CC...), func(s string) bool { return slices.Contains(public, s) }),
		Published:    parseTime(n.Published),
		Updated:      parseTime(n.Updated),
	}

	for _, a := range n.Attachment {
		doc.Attachments = append(doc.Attachments, fedi.AttachmentDoc{
			URL: a.URL.first(), MediaType: a.MediaType, Description: a.Name,
			Cover: imageURL(a.Icon), Blurhash: a.Blurhash, Width: a.Width, Height: a.Height,
			Duration: parseDuration(a.Duration),
		})
	}

	return doc
}

// imageURL is the address of an image object, or of the first of a list
// of them; Mastodon gives the cover a video's author chose so.
func imageURL(raw json.RawMessage) string {
	var image struct {
		URL flexible `json:"url"`
	}
	if json.Unmarshal(raw, &image) == nil {
		return image.URL.first()
	}

	var images []json.RawMessage
	if json.Unmarshal(raw, &images) == nil && len(images) > 0 {
		return imageURL(images[0])
	}

	return ""
}

// isoDuration is an ISO 8601 duration of hours, minutes and seconds, as
// Mastodon ("PT1M24.5S") and PeerTube ("PT84S") write it.
var isoDuration = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?$`)

// parseDuration is zero for a duration it cannot read.
func parseDuration(s string) time.Duration {
	m := isoDuration.FindStringSubmatch(s)
	if m == nil {
		return 0
	}

	var d time.Duration
	for i, unit := range []time.Duration{time.Hour, time.Minute, time.Second} {
		if m[i+1] == "" {
			continue
		}

		n, err := strconv.ParseFloat(m[i+1], 64)
		if err != nil {
			return 0
		}
		d += time.Duration(n * float64(unit))
	}

	return d
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}

	return t
}

// PlainText is the HTML of a note as the plain text of a post here: lines
// and paragraphs kept, a link written as its whole address (servers hide
// parts of it), a mention or a hashtag as its words, nothing else of the
// markup.
func PlainText(source string) string {
	var (
		b       strings.Builder
		z       = html.NewTokenizer(strings.NewReader(source))
		inLink  int
		skipped int
	)

	for {
		switch z.Next() {
		case html.ErrorToken:
			return tidy(b.String())
		case html.TextToken:
			if inLink == 0 && skipped == 0 {
				b.Write(z.Text())
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			attrs := attributes(z)

			switch string(name) {
			case "br":
				b.WriteString("\n")
			case "p":
				b.WriteString("\n\n")
			case "script", "style":
				skipped++
			case "a":
				if class := attrs["class"]; strings.Contains(class, "mention") || strings.Contains(class, "hashtag") {
					continue
				}
				if href := attrs["href"]; href != "" {
					b.WriteString(href)
				}
				inLink++
			}
		case html.EndTagToken:
			name, _ := z.TagName()

			switch string(name) {
			case "p":
				b.WriteString("\n\n")
			case "script", "style":
				skipped = max(skipped-1, 0)
			case "a":
				inLink = max(inLink-1, 0)
			}
		}
	}
}

func attributes(z *html.Tokenizer) map[string]string {
	attrs := map[string]string{}

	for {
		key, value, more := z.TagAttr()
		if len(key) > 0 {
			attrs[string(key)] = string(value)
		}
		if !more {
			return attrs
		}
	}
}

// tidy trims the lines of a text and keeps at most one blank line in a row.
func tidy(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, line)
	}

	return strings.TrimSpace(strings.Join(out, "\n"))
}
