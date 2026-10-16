package post

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxLinkURL         = 2048
	maxLinkTitle       = 256
	maxLinkDescription = 512
	maxLinkSite        = 128
)

var ErrInvalidLink = errors.New("invalid link preview")

// Link is the card of the page a post links to: its address, title,
// description and site, as the page tells them (og:title and the like),
// and a picture of it kept by media as a photo of the post's author. The
// gateway reads the page; post only keeps what it was told.
type Link struct {
	url         string
	title       string
	description string
	site        string
	// photoID is the picture in media, uuid.Nil for none.
	photoID uuid.UUID
}

// LinkDraft is what a card is made of.
type LinkDraft struct {
	URL         string
	Title       string
	Description string
	Site        string
	PhotoID     uuid.UUID
}

// NewLink checks a card: an http or https address with a host and a
// title, each part on one line and within its limit.
func NewLink(d LinkDraft) (Link, error) {
	u, err := url.Parse(d.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(d.URL) > maxLinkURL {
		return Link{}, ErrInvalidLink
	}

	title, description, site := strings.TrimSpace(d.Title), strings.TrimSpace(d.Description), strings.TrimSpace(d.Site)

	if title == "" || !fits(title, maxLinkTitle) || !fits(description, maxLinkDescription) || !fits(site, maxLinkSite) {
		return Link{}, ErrInvalidLink
	}

	return Link{url: d.URL, title: title, description: description, site: site, photoID: d.PhotoID}, nil
}

// fits tells a line of at most limit characters.
func fits(s string, limit int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= limit && !strings.ContainsAny(s, "\r\n")
}

// RestoreLink rebuilds a card from storage without validation.
func RestoreLink(d LinkDraft) Link {
	return Link{url: d.URL, title: d.Title, description: d.Description, site: d.Site, photoID: d.PhotoID}
}

// IsZero tells no card.
func (l Link) IsZero() bool {
	return l.url == ""
}

func (l Link) URL() string {
	return l.url
}

func (l Link) Title() string {
	return l.title
}

func (l Link) Description() string {
	return l.description
}

func (l Link) Site() string {
	return l.site
}

func (l Link) PhotoID() uuid.UUID {
	return l.photoID
}
