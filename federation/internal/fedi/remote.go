package fedi

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxRemoteNameLen     = 100
	maxRemoteUsernameLen = 64
)

// Remote is an actor of another server as its document tells it: where
// to send to it and the key it signs with. The ID, the inbox and the key
// are on the actor's server, or the document is not believed.
type Remote struct {
	id          URL
	inbox       URL
	sharedInbox URL
	keyID       URL
	publicKey   string
	profile     Profile
	fetchedAt   time.Time
}

// Profile is how a remote actor shows itself: its name, its user name on
// its server, its photo and its page there. The photo and the page may be
// on another host, as media often are; any of them may be empty.
type Profile struct {
	Name     string
	Username string
	Icon     URL
	Page     URL
}

// RemoteDoc is what an actor's document says, before it is checked.
type RemoteDoc struct {
	ID          string
	Inbox       string
	SharedInbox string
	KeyID       string
	PublicKey   string
	Name        string
	Username    string
	Icon        string
	Page        string
}

// NewRemote checks the document of the actor fetched from from: it must
// be the actor asked for, and point nowhere but at its own server. A
// shared inbox elsewhere is dropped rather than trusted.
func NewRemote(from URL, doc RemoteDoc, now time.Time) (*Remote, error) {
	id, err := NewURL(doc.ID)
	if err != nil || id != from.WithoutFragment() {
		return nil, ErrInvalidActivity
	}

	inbox, err := NewURL(doc.Inbox)
	if err != nil || inbox.Host() != id.Host() {
		return nil, ErrInvalidActivity
	}

	keyID, err := NewURL(doc.KeyID)
	if err != nil || keyID.Host() != id.Host() || doc.PublicKey == "" {
		return nil, ErrInvalidActivity
	}

	shared, err := NewURL(doc.SharedInbox)
	if err != nil || shared.Host() != id.Host() {
		shared = ""
	}

	return &Remote{
		id:          id,
		inbox:       inbox,
		sharedInbox: shared,
		keyID:       keyID,
		publicKey:   doc.PublicKey,
		profile:     newProfile(doc),
		fetchedAt:   now.UTC(),
	}, nil
}

// newProfile keeps what of the profile is sound: a name or a user name
// too long is cut, an address that is not https dropped.
func newProfile(doc RemoteDoc) Profile {
	p := Profile{
		Name:     cut(strings.TrimSpace(doc.Name), maxRemoteNameLen),
		Username: cut(strings.TrimSpace(doc.Username), maxRemoteUsernameLen),
	}

	if icon, err := NewURL(doc.Icon); err == nil {
		p.Icon = icon
	}

	if page, err := NewURL(doc.Page); err == nil {
		p.Page = page
	}

	return p
}

// cut is s cut to at most n characters.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}

	return string([]rune(s)[:n])
}

func RestoreRemote(id, inbox, sharedInbox, keyID URL, publicKey string, profile Profile, fetchedAt time.Time) *Remote {
	return &Remote{
		id:          id,
		inbox:       inbox,
		sharedInbox: sharedInbox,
		keyID:       keyID,
		publicKey:   publicKey,
		profile:     profile,
		fetchedAt:   fetchedAt,
	}
}

func (r *Remote) Profile() Profile {
	return r.profile
}

// Handle is the address people know the actor by: @username@host, or
// its ID without a user name.
func (r *Remote) Handle() string {
	if r.profile.Username == "" {
		return r.id.String()
	}

	return "@" + r.profile.Username + "@" + r.id.Host()
}

func (r *Remote) ID() URL {
	return r.id
}

func (r *Remote) Inbox() URL {
	return r.inbox
}

func (r *Remote) SharedInbox() URL {
	return r.sharedInbox
}

// DeliveryInbox is where to send what its followers get: the shared
// inbox of its server when there is one.
func (r *Remote) DeliveryInbox() URL {
	if r.sharedInbox != "" {
		return r.sharedInbox
	}

	return r.inbox
}

func (r *Remote) KeyID() URL {
	return r.keyID
}

func (r *Remote) PublicKey() string {
	return r.publicKey
}

func (r *Remote) FetchedAt() time.Time {
	return r.fetchedAt
}

// Stale: the document is old enough to fetch again, as keys and inboxes
// change.
func (r *Remote) Stale(now time.Time, maxAge time.Duration) bool {
	return now.Sub(r.fetchedAt) > maxAge
}
