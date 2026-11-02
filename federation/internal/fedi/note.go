package fedi

import (
	"html"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Person is the document of a page open to the Fediverse.
type Person struct {
	ID        URL
	Page      URL
	Nickname  string
	Name      string
	Summary   string
	Inbox     URL
	Outbox    URL
	Followers URL
	Shared    URL
	KeyID     URL
	PublicKey string
	// Avatar is empty without a photo.
	Avatar    URL
	Published time.Time
	// Service: the actor is the site itself, not a person.
	Service bool
}

// Note is a post as the Fediverse sees it.
type Note struct {
	ID           URL
	Page         URL
	AttributedTo URL
	// Content is HTML, made from the plain text of the post.
	Content   string
	Published time.Time
	// Updated is zero for a post never edited.
	Updated     time.Time
	Sensitive   bool
	Attachments []URL
	To          []string
	CC          []string
}

// Post is what post tells of a post, as much as the Fediverse needs.
type Post struct {
	ID       uuid.UUID
	AuthorID uuid.UUID
	WallID   uuid.UUID
	Text     string
	PhotoIDs []uuid.UUID
	Created  time.Time
	Edited   time.Time
	NSFW     bool
	FromWall bool
	Repost   bool
	Paid     bool
	Archived bool
}

// Federates: the post goes to the Fediverse. Only what the author writes
// on their own wall does, and only what anyone may see in full: not a
// repost, whose original may be anyone's, nor one whose photos are paid
// for, nor one put away in the archive.
func (p Post) Federates() bool {
	return p.WallID == p.AuthorID && !p.FromWall && !p.Repost && !p.Paid && !p.Archived
}

// Note is the post p of the page at site.
func (p Post) Note(site Site) Note {
	attachments := make([]URL, len(p.PhotoIDs))
	for i, id := range p.PhotoIDs {
		attachments[i] = site.Photo(p.AuthorID, id)
	}

	return Note{
		ID:           site.Note(p.ID),
		Page:         site.PostPage(p.ID),
		AttributedTo: site.Actor(p.AuthorID),
		Content:      TextHTML(p.Text, site),
		Published:    p.Created,
		Updated:      p.Edited,
		Sensitive:    p.NSFW,
		Attachments:  attachments,
		To:           []string{Public},
		CC:           []string{site.Followers(p.AuthorID).String()},
	}
}

var (
	// mention is how a text names a person: [nickname|Name], the
	// pattern of the site (mentionPattern of labels.js).
	mention = regexp.MustCompile(`\[([A-Za-z][A-Za-z0-9_]*)\|([^\[\]|\n]{1,100})\]`)
	// link is a web address in a text, as the site links it.
	link = regexp.MustCompile(`https?://[^\s<>"]+`)
)

// TextHTML is the plain text of a post as HTML: escaped, paragraphs for
// blank lines, line breaks, and links for web addresses and the people
// it names, which lead to their pages here.
func TextHTML(text string, site Site) string {
	var paragraphs []string

	for _, part := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		part = strings.Trim(part, "\n")
		if strings.TrimSpace(part) == "" {
			continue
		}

		lines := strings.Split(part, "\n")
		for i, line := range lines {
			lines[i] = lineHTML(line, site)
		}

		paragraphs = append(paragraphs, "<p>"+strings.Join(lines, "<br>")+"</p>")
	}

	return strings.Join(paragraphs, "")
}

// lineHTML escapes a line and links what it names and its addresses.
func lineHTML(line string, site Site) string {
	var b strings.Builder

	rest := line
	for {
		loc := mention.FindStringSubmatchIndex(rest)
		if loc == nil {
			b.WriteString(linksHTML(rest))
			return b.String()
		}

		b.WriteString(linksHTML(rest[:loc[0]]))

		nickname, name := rest[loc[2]:loc[3]], rest[loc[4]:loc[5]]
		b.WriteString(`<a href="` + html.EscapeString(site.ProfilePage(nickname).String()) + `">` + html.EscapeString(name) + `</a>`)

		rest = rest[loc[1]:]
	}
}

// linksHTML escapes s and links its web addresses, without the marks a
// sentence ends with.
func linksHTML(s string) string {
	var b strings.Builder

	last := 0
	for _, loc := range link.FindAllStringIndex(s, -1) {
		end := loc[1]
		for end > loc[0] {
			r, size := utf8.DecodeLastRuneInString(s[loc[0]:end])
			if !strings.ContainsRune(".,;:!?)»'", r) {
				break
			}
			end -= size
		}

		address := html.EscapeString(s[loc[0]:end])
		b.WriteString(html.EscapeString(s[last:loc[0]]))
		b.WriteString(`<a href="` + address + `" rel="nofollow noopener noreferrer" target="_blank">` + address + `</a>`)
		last = end
	}

	b.WriteString(html.EscapeString(s[last:]))

	return b.String()
}
