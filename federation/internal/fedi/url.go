// Package fedi is the domain of federation: the pages of the site seen by
// other servers of the Fediverse (ActivityPub), the people there who
// follow them, and the activities that go between the servers.
package fedi

import (
	"errors"
	"net/url"
	"strings"
)

const maxURLLen = 2048

var (
	ErrInvalidURL = errors.New("invalid url")
	ErrNotFound   = errors.New("not found")
	// ErrNotFederated: the page is not open to the Fediverse.
	ErrNotFederated = errors.New("the page is not open to the fediverse")
	// ErrPageHidden: a page only some may see cannot be opened to the
	// Fediverse, where anyone reads it.
	ErrPageHidden = errors.New("a hidden page cannot be opened to the fediverse")
	// ErrGone: the remote server says the object is deleted for good.
	ErrGone = errors.New("the remote object is gone")
	// ErrBadSignature: the request is not signed by whom it says.
	ErrBadSignature = errors.New("bad signature")
	// ErrInvalidActivity: the activity is not one we can read.
	ErrInvalidActivity = errors.New("invalid activity")
	// ErrTooMany: a remote server sends more than it may.
	ErrTooMany = errors.New("too many activities")
)

// URL is an absolute https address of an object of the Fediverse: an
// actor, an inbox, an activity. Other schemes and user names are not.
type URL string

func NewURL(s string) (URL, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxURLLen {
		return "", ErrInvalidURL
	}

	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", ErrInvalidURL
	}

	return URL(s), nil
}

func (u URL) String() string {
	return string(u)
}

// Host is the server of the address, with its port if any.
func (u URL) Host() string {
	parsed, err := url.Parse(string(u))
	if err != nil {
		return ""
	}

	return strings.ToLower(parsed.Host)
}

// WithoutFragment is the address of the document a fragment is in: the
// actor whose key is "…#main-key".
func (u URL) WithoutFragment() URL {
	s, _, _ := strings.Cut(string(u), "#")
	return URL(s)
}
