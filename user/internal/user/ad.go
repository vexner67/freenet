package user

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// MaxAdURLLength is the longest link an ad leads to.
	MaxAdURLLength = 2048
	// MaxAdTitleLength is the longest title of an ad, one line.
	MaxAdTitleLength = 100
	// MaxAdTextLength is the longest text under the title.
	MaxAdTextLength = 500
	// MaxAds is how many ads the site keeps, shown or not.
	MaxAds = 100
)

var (
	ErrAdURLInvalid   = errors.New("ad link must be an http or https link")
	ErrAdURLTooLong   = errors.New("ad link is too long")
	ErrAdTitleEmpty   = errors.New("ad title is empty")
	ErrAdTitleTooLong = errors.New("ad title is too long")
	ErrAdTextTooLong  = errors.New("ad text is too long")
	ErrAdNotFound     = errors.New("ad not found")
	ErrTooManyAds     = errors.New("too many ads")
	ErrAdPhotoNoOwner = errors.New("ad photo needs its owner")
)

// AdURL is where an ad leads: an http or https link, https:// added when
// the scheme is left out, as the "Сайт" of a community.
type AdURL string

func NewAdURL(s string) (AdURL, error) {
	s = strings.TrimSpace(s)
	if s != "" && !strings.Contains(s, "://") {
		s = "https://" + s
	}

	if utf8.RuneCountInString(s) > MaxAdURLLength {
		return "", ErrAdURLTooLong
	}

	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		strings.ContainsAny(s, " \t\n\r") {
		return "", ErrAdURLInvalid
	}

	return AdURL(s), nil
}

func (u AdURL) String() string {
	return string(u)
}

// AdTitle is the bold first line of an ad.
type AdTitle string

func NewAdTitle(s string) (AdTitle, error) {
	s, err := visibleText(strings.Join(strings.Fields(s), " "))
	if err != nil {
		return "", err
	}

	switch {
	case s == "":
		return "", ErrAdTitleEmpty
	case utf8.RuneCountInString(s) > MaxAdTitleLength:
		return "", ErrAdTitleTooLong
	}

	return AdTitle(s), nil
}

func (t AdTitle) String() string {
	return string(t)
}

// AdText is what an ad says under its title, possibly nothing.
type AdText string

func NewAdText(s string) (AdText, error) {
	s, err := visibleText(s)
	if err != nil {
		return "", err
	}

	if utf8.RuneCountInString(s) > MaxAdTextLength {
		return "", ErrAdTextTooLong
	}

	return AdText(s), nil
}

func (t AdText) String() string {
	return string(t)
}

// AdPhoto is the cover of an ad: a photo in media, kept under the
// administrator who uploaded it. The zero value is no cover.
type AdPhoto struct {
	OwnerID uuid.UUID
	PhotoID uuid.UUID
}

// IsZero tells that the ad has no cover.
func (p AdPhoto) IsZero() bool {
	return p.PhotoID == uuid.Nil
}

// Ad is a promoted post of the global feed, as Twitter slips its promoted
// tweets between the others: a link with a title, a text and a cover,
// which the administrators of the site make and show or hide.
type Ad struct {
	id     uuid.UUID
	url    AdURL
	title  AdTitle
	text   AdText
	photo  AdPhoto
	active bool
}

// AdDraft is what a new ad says.
type AdDraft struct {
	URL    AdURL
	Title  AdTitle
	Text   AdText
	Photo  AdPhoto
	Active bool
}

func NewAd(d AdDraft) (*Ad, error) {
	if !d.Photo.IsZero() && d.Photo.OwnerID == uuid.Nil {
		return nil, ErrAdPhotoNoOwner
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Ad{id: id, url: d.URL, title: d.Title, text: d.Text, photo: d.Photo, active: d.Active}, nil
}

// RestoreAd is an ad as storage keeps it.
func RestoreAd(id uuid.UUID, d AdDraft) *Ad {
	return &Ad{id: id, url: d.URL, title: d.Title, text: d.Text, photo: d.Photo, active: d.Active}
}

// AdChange is what an edit says; nil leaves a part as it is, a zero
// Photo takes the cover away and the cover the ad has already keeps it.
type AdChange struct {
	URL    *AdURL
	Title  *AdTitle
	Text   *AdText
	Photo  *AdPhoto
	Active *bool
}

// Edited is the ad as the change makes it and the cover it no longer
// shows (zero when it keeps it).
func (a *Ad) Edited(c AdChange) (*Ad, AdPhoto, error) {
	edited := *a

	if c.URL != nil {
		edited.url = *c.URL
	}

	if c.Title != nil {
		edited.title = *c.Title
	}

	if c.Text != nil {
		edited.text = *c.Text
	}

	if c.Active != nil {
		edited.active = *c.Active
	}

	var dropped AdPhoto

	if c.Photo != nil && c.Photo.PhotoID != a.photo.PhotoID {
		if !c.Photo.IsZero() && c.Photo.OwnerID == uuid.Nil {
			return nil, AdPhoto{}, ErrAdPhotoNoOwner
		}

		edited.photo = *c.Photo
		dropped = a.photo
	}

	return &edited, dropped, nil
}

func (a *Ad) ID() uuid.UUID  { return a.id }
func (a *Ad) URL() AdURL     { return a.url }
func (a *Ad) Title() AdTitle { return a.title }
func (a *Ad) Text() AdText   { return a.text }
func (a *Ad) Photo() AdPhoto { return a.photo }
func (a *Ad) IsActive() bool { return a.active }

// AdEvent is what a reader did with an ad: saw it in the feed or followed
// its link.
type AdEvent string

const (
	AdViewed  AdEvent = "view"
	AdClicked AdEvent = "click"
)

// AdStats is how an ad does: how often it was seen and followed, and by
// how many people.
type AdStats struct {
	Views    int64
	Viewers  int64
	Clicks   int64
	Clickers int64
}
