package fedi

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Site is where the site is on the web, and the addresses it gives its
// objects. An actor is known by the ID of the user, which never changes,
// not by the nickname, which does.
type Site struct {
	base *url.URL
}

// NewSite is the site at base, e.g. https://freenet19.fun. Other servers
// reach only https; http is for local development.
func NewSite(base string) (Site, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(base), "/"))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.Path != "" || u.User != nil {
		return Site{}, ErrInvalidURL
	}

	return Site{base: u}, nil
}

// Domain is the host of the site, which addresses of people end with:
// @nickname@domain.
func (s Site) Domain() string {
	return strings.ToLower(s.base.Host)
}

func (s Site) at(path string) URL {
	return URL(s.base.String() + path)
}

// Actor is the actor of a page; uuid.Nil is the actor of the site
// itself, which follows the accounts of other servers for the news.
func (s Site) Actor(userID uuid.UUID) URL {
	if userID == uuid.Nil {
		return s.at("/ap/actor")
	}

	return s.at("/ap/users/" + userID.String())
}

func (s Site) Key(userID uuid.UUID) URL {
	return s.Actor(userID) + "#main-key"
}

func (s Site) Inbox(userID uuid.UUID) URL {
	return s.Actor(userID) + "/inbox"
}

func (s Site) Outbox(userID uuid.UUID) URL {
	return s.Actor(userID) + "/outbox"
}

func (s Site) Followers(userID uuid.UUID) URL {
	return s.Actor(userID) + "/followers"
}

// SharedInbox takes what a server sends to several people here at once.
func (s Site) SharedInbox() URL {
	return s.at("/ap/inbox")
}

func (s Site) Note(postID uuid.UUID) URL {
	return s.at("/ap/posts/" + postID.String())
}

// Activity is an activity about a note, told apart by kind and time: an
// edit at another time is another activity.
func (s Site) Activity(postID uuid.UUID, kind string, at time.Time) URL {
	return s.Note(postID) + URL("#"+strings.ToLower(kind)+"-"+strconv.FormatInt(at.UnixMilli(), 10))
}

// Accept is the activity accepting a follow, one of its own per follow.
func (s Site) Accept(userID, id uuid.UUID) URL {
	return s.Actor(userID) + URL("#accept-"+id.String())
}

// Follow is a follow the site sends, one of its own per follow; Unfollow
// undoes it.
func (s Site) Follow(id uuid.UUID) URL {
	return s.Actor(uuid.Nil) + URL("#follow-"+id.String())
}

func (s Site) Unfollow(id uuid.UUID) URL {
	return s.Actor(uuid.Nil) + URL("#undo-"+id.String())
}

// Deletion is the activity that deletes the actor for the Fediverse.
func (s Site) Deletion(userID uuid.UUID, at time.Time) URL {
	return s.Actor(userID) + URL("#delete-"+strconv.FormatInt(at.UnixMilli(), 10))
}

// ProfilePage and PostPage are the pages people open in a browser.
func (s Site) ProfilePage(nickname string) URL {
	return s.at("/" + url.PathEscape(nickname))
}

func (s Site) PostPage(postID uuid.UUID) URL {
	return s.at("/post/" + postID.String())
}

func (s Site) Photo(ownerID, photoID uuid.UUID) URL {
	return s.at("/media/photos/" + ownerID.String() + "/" + photoID.String() + "/large.jpg")
}

func (s Site) Avatar(userID uuid.UUID, avatarID string) URL {
	return s.at("/media/avatars/" + userID.String() + "/" + url.PathEscape(avatarID) + "/large.jpg")
}

// UserOf is the user whose actor address u is, if it is one of ours:
// uuid.Nil for the actor of the site.
func (s Site) UserOf(u URL) (uuid.UUID, bool) {
	if u == s.Actor(uuid.Nil) {
		return uuid.Nil, true
	}

	rest, ok := strings.CutPrefix(string(u), string(s.at("/ap/users/")))
	if !ok {
		return uuid.Nil, false
	}

	id, err := uuid.Parse(rest)
	if err != nil {
		return uuid.Nil, false
	}

	return id, true
}

// Nickname is whom a WebFinger resource names here: acct:nick@domain,
// nick@domain or the page https://domain/nick.
func (s Site) Nickname(resource string) (string, bool) {
	resource = strings.TrimSpace(resource)

	if page, ok := strings.CutPrefix(resource, s.base.String()+"/"); ok {
		if page == "" || strings.ContainsAny(page, "/?#") {
			return "", false
		}

		return page, true
	}

	acct := strings.TrimPrefix(strings.TrimPrefix(resource, "acct:"), "@")

	nickname, domain, ok := strings.Cut(acct, "@")
	if !ok || nickname == "" || !strings.EqualFold(domain, s.Domain()) {
		return "", false
	}

	return nickname, true
}
