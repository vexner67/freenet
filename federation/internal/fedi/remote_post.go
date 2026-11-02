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
	maxVideos        = 4
	maxVideoSide     = 10000
	maxVideoDuration = 24 * time.Hour
	maxDescription   = 1500
	maxLinkTitle     = 300
	maxLinkSite      = 100
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
	// Link is the card of the page the note links to, when its server
	// tells one.
	Link LinkDoc
}

// LinkDoc is the card of a page as a server tells it.
type LinkDoc struct {
	URL         string
	Site        string
	Title       string
	Description string
	Image       string
}

type AttachmentDoc struct {
	URL         string
	MediaType   string
	Description string
	// Cover is the picture the author chose for a video.
	Cover    string
	Blurhash string
	Width    int
	Height   int
	Duration time.Duration
}

// Image is a picture of a remote post, on its server.
type Image struct {
	URL         URL
	Description string
}

// Video is a video of a remote post, on its server. The site shows its
// cover, which opens the post on its server.
type Video struct {
	URL URL
	// Cover is the picture its author chose for it; without one the site
	// cuts the first frame.
	Cover URL
	// Blurhash is a blurred cover, drawn until the cover is read.
	Blurhash    string
	Width       int
	Height      int
	Duration    time.Duration
	Description string
}

// Link is the card of the page a remote post links to: its picture is on
// the server of the post. The zero Link is none.
type Link struct {
	URL         URL
	Site        string
	Title       string
	Description string
	Image       URL
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
	videos    []Video
	link      Link
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

// Reread is the post as a page read again tells it, and whether anything
// changed. Only another text or another number of pictures and videos is
// an edit: the addresses of pictures on such pages change by themselves.
func (p *RemotePost) Reread(doc NoteDoc, now time.Time) (*RemotePost, bool, error) {
	again, err := p.Edited(doc, now)
	if err != nil {
		return nil, false, err
	}

	if again.text == p.text && again.summary == p.summary && len(again.images) == len(p.images) && len(again.videos) == len(p.videos) {
		again.updated = p.updated
	}

	changed := again.text != p.text || again.summary != p.summary || again.page != p.page || !again.updated.Equal(p.updated) ||
		!slices.Equal(again.images, p.images) || !slices.Equal(again.videos, p.videos) || again.link != p.link

	return again, changed, nil
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
	p.videos = videos(doc.Attachments)
	p.link = linkCard(doc.Link)

	p.page = ""
	if page, err := NewURL(doc.URL); err == nil {
		p.page = page
	}

	if p.text == "" && len(p.images) == 0 && len(p.videos) == 0 {
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

// linkCard is the card of a page with an address and a title, or none.
func linkCard(d LinkDoc) Link {
	u, err := NewURL(d.URL)
	title := cut(strings.TrimSpace(d.Title), maxLinkTitle)
	if err != nil || title == "" {
		return Link{}
	}

	l := Link{
		URL:         u,
		Site:        cut(strings.TrimSpace(d.Site), maxLinkSite),
		Title:       title,
		Description: cut(strings.TrimSpace(d.Description), maxDescription),
	}
	if image, err := NewURL(d.Image); err == nil {
		l.Image = image
	}

	return l
}

// videos are the videos among the attachments, at most four.
func videos(docs []AttachmentDoc) []Video {
	var out []Video

	for _, d := range docs {
		u, err := NewURL(d.URL)
		if err != nil || !strings.HasPrefix(d.MediaType, "video/") {
			continue
		}

		v := Video{URL: u, Description: cut(strings.TrimSpace(d.Description), maxDescription)}
		if cover, err := NewURL(d.Cover); err == nil {
			v.Cover = cover
		}
		if validBlurhash(d.Blurhash) {
			v.Blurhash = d.Blurhash
		}
		if d.Width > 0 && d.Width <= maxVideoSide && d.Height > 0 && d.Height <= maxVideoSide {
			v.Width, v.Height = d.Width, d.Height
		}
		if d.Duration > 0 && d.Duration <= maxVideoDuration {
			v.Duration = d.Duration
		}

		out = append(out, v)
		if len(out) == maxVideos {
			break
		}
	}

	return out
}

const base83 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~"

// validBlurhash: s is a blurhash a browser can draw. Its first character
// tells how many components follow, and so its length.
func validBlurhash(s string) bool {
	if len(s) < 6 || strings.Trim(s, base83) != "" {
		return false
	}

	size := strings.IndexByte(base83, s[0])
	x, y := size%9+1, size/9+1

	return len(s) == 4+2*x*y
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

func RestoreRemotePost(id uuid.UUID, uri, actor, page URL, text, summary string, images []Image, videos []Video, sensitive bool, language string, published, updated time.Time) *RemotePost {
	return &RemotePost{
		id: id, uri: uri, actor: actor, page: page, text: text, summary: summary, images: images, videos: videos,
		sensitive: sensitive, language: language, published: published, updated: updated,
	}
}

// WithLink is the post restored with the card of the page it links to.
func (p *RemotePost) WithLink(l Link) *RemotePost {
	with := *p
	with.link = l

	return &with
}

func (p *RemotePost) ID() uuid.UUID        { return p.id }
func (p *RemotePost) URI() URL             { return p.uri }
func (p *RemotePost) Actor() URL           { return p.actor }
func (p *RemotePost) Text() string         { return p.text }
func (p *RemotePost) Summary() string      { return p.summary }
func (p *RemotePost) Images() []Image      { return slices.Clone(p.images) }
func (p *RemotePost) Videos() []Video      { return slices.Clone(p.videos) }
func (p *RemotePost) Link() Link           { return p.link }
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
