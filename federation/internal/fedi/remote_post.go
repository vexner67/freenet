package fedi

import (
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// maxRemoteTextLen is the limit of a post here: a longer note is cut.
	maxRemoteTextLen = 60000
	maxSummaryLen    = 500
	maxImages        = 4
	maxDescription   = 1500
	maxLanguageLen   = 8
)

// ErrNotForNews: the note is sound, but not one the news show: a reply,
// one not addressed to everyone or one with nothing to show.
var ErrNotForNews = errors.New("the note is not for the news")

// NoteDoc is what a note of another server says, its HTML already made
// plain text, before it is checked.
type NoteDoc struct {
	ID           string
	AttributedTo string
	URL          string
	InReplyTo    string
	Text         string
	// Summary is the warning shown before the text, "content warning".
	Summary   string
	Language  string
	Sensitive bool
	// Public: addressed to everyone.
	Public      bool
	Published   time.Time
	Updated     time.Time
	Attachments []AttachmentDoc
}

type AttachmentDoc struct {
	URL         string
	MediaType   string
	Description string
}

// Image is a picture of a remote post, on its server.
type Image struct {
	URL         URL
	Description string
}

// RemotePost is a note of an account of another server the site follows,
// kept for the news. Its ID is a UUIDv7 of the time it was published, so
// it takes its place among the posts of the site.
type RemotePost struct {
	id        uuid.UUID
	uri       URL
	actor     URL
	page      URL
	text      string
	summary   string
	images    []Image
	sensitive bool
	language  string
	published time.Time
	updated   time.Time
}

// NewRemotePost checks a note actor sent: it must be the actor's, on its
// server, and for everyone.
func NewRemotePost(doc NoteDoc, actor URL, now time.Time) (*RemotePost, error) {
	uri, err := NewURL(doc.ID)
	if err != nil || uri.Host() != actor.Host() || URL(doc.AttributedTo) != actor {
		return nil, ErrInvalidActivity
	}

	published := doc.Published.UTC()
	if published.IsZero() || published.After(now) {
		published = now.UTC()
	}

	p := &RemotePost{uri: uri, actor: actor, published: published}
	if err = p.fill(doc, now); err != nil {
		return nil, err
	}

	if p.id, err = idAt(published); err != nil {
		return nil, err
	}

	return p, nil
}

// Edited is the post as the note doc tells it now.
func (p *RemotePost) Edited(doc NoteDoc, now time.Time) (*RemotePost, error) {
	if URL(doc.ID) != p.uri || URL(doc.AttributedTo) != p.actor {
		return nil, ErrInvalidActivity
	}

	edited := *p
	if err := edited.fill(doc, now); err != nil {
		return nil, err
	}

	edited.updated = now.UTC()
	if !doc.Updated.IsZero() && doc.Updated.Before(now) {
		edited.updated = doc.Updated.UTC()
	}

	return &edited, nil
}

func (p *RemotePost) fill(doc NoteDoc, now time.Time) error {
	if !doc.Public || doc.InReplyTo != "" {
		return ErrNotForNews
	}

	p.text = cut(strings.TrimSpace(doc.Text), maxRemoteTextLen)
	p.summary = cut(strings.TrimSpace(doc.Summary), maxSummaryLen)
	p.sensitive = doc.Sensitive || p.summary != ""
	p.language = language(doc.Language)
	p.images = images(doc.Attachments)

	p.page = ""
	if page, err := NewURL(doc.URL); err == nil {
		p.page = page
	}

	if p.text == "" && len(p.images) == 0 {
		return ErrNotForNews
	}

	return nil
}

// images are the pictures among the attachments, at most four, as many
// as a post here has room for; video and sound are left out.
func images(docs []AttachmentDoc) []Image {
	var out []Image

	for _, d := range docs {
		u, err := NewURL(d.URL)
		if err != nil || !strings.HasPrefix(d.MediaType, "image/") {
			continue
		}

		out = append(out, Image{URL: u, Description: cut(strings.TrimSpace(d.Description), maxDescription)})
		if len(out) == maxImages {
			break
		}
	}

	return out
}

// language is the primary language tag of a note: "ru" of "ru-RU".
func language(tag string) string {
	tag, _, _ = strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
	if utf8.RuneCountInString(tag) > maxLanguageLen {
		return ""
	}

	return tag
}

// idAt is a UUIDv7 whose time is t.
func idAt(t time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	if _, err := rand.Read(id[:]); err != nil {
		return uuid.Nil, err
	}

	ms := uint64(t.UnixMilli())
	for i := 0; i < 6; i++ {
		id[i] = byte(ms >> (40 - 8*i))
	}

	id[6] = id[6]&0x0f | 0x70 // version 7
	id[8] = id[8]&0x3f | 0x80 // RFC 9562 variant

	return id, nil
}

func RestoreRemotePost(id uuid.UUID, uri, actor, page URL, text, summary string, images []Image, sensitive bool, language string, published, updated time.Time) *RemotePost {
	return &RemotePost{
		id: id, uri: uri, actor: actor, page: page, text: text, summary: summary, images: images,
		sensitive: sensitive, language: language, published: published, updated: updated,
	}
}

func (p *RemotePost) ID() uuid.UUID        { return p.id }
func (p *RemotePost) URI() URL             { return p.uri }
func (p *RemotePost) Actor() URL           { return p.actor }
func (p *RemotePost) Text() string         { return p.text }
func (p *RemotePost) Summary() string      { return p.summary }
func (p *RemotePost) Images() []Image      { return slices.Clone(p.images) }
func (p *RemotePost) Sensitive() bool      { return p.sensitive }
func (p *RemotePost) Language() string     { return p.language }
func (p *RemotePost) Published() time.Time { return p.published }
func (p *RemotePost) Updated() time.Time   { return p.updated }

// Page is where people read the note on its server: its URL, or its ID.
func (p *RemotePost) Page() URL {
	if p.page != "" {
		return p.page
	}

	return p.uri
}

// Speaks: the post is in one of languages, or tells none.
func (p *RemotePost) Speaks(languages []string) bool {
	return p.language == "" || slices.Contains(languages, p.language)
}
