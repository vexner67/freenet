package model

import "time"

// Federation is whether a page is open to the Fediverse: open, how many
// people of other servers follow it and the domain its address there ends
// with (@nickname@domain).
type Federation struct {
	Open      bool
	Followers int
	Domain    string
}

// FederationDocument is an ActivityPub document, passed on as it is, and
// the page a browser is sent to instead.
type FederationDocument struct {
	Body    []byte
	PageURL string
}

// InboxRequest is a request another server made to an inbox, as it came:
// its signature covers the method, the path and the headers.
type InboxRequest struct {
	Method string
	Path   string
	// Headers have their names in lower case.
	Headers map[string]string
	Body    []byte
	// UserID is the page whose inbox it is, "" for the shared inbox.
	UserID string
}

// FediverseAccount is an account of another server of the Fediverse.
type FediverseAccount struct {
	// ID is the address of its actor.
	ID string
	// Handle is @username@host.
	Handle  string
	Name    string
	PageURL string
	// HasAvatar: it has a photo, which the site shows through itself.
	HasAvatar bool
	// Accepted: its server took the follow of the site; until then its
	// posts may not come.
	Accepted bool
	// Telegram: a public channel of Telegram, which the site reads from
	// its page rather than the Fediverse.
	Telegram bool
}

// FediverseLookup is an account found by its address, and whether the
// caller reads it.
type FediverseLookup struct {
	Account    FediverseAccount
	Subscribed bool
}

// FediverseNews asks for remote posts of the news: of the accounts in the
// news of everyone (Featured) or of those the caller reads.
type FediverseNews struct {
	Featured      bool
	Before        string
	Limit         int
	HideSensitive bool
}

// FediversePost is a post of an account of another server in the news.
// Its ID is a UUIDv7 of the time it was published, so it sorts with the
// posts of the site.
type FediversePost struct {
	ID      string
	URL     string
	PageURL string
	Author  FediverseAccount
	// Text is plain text, made from the HTML of the note.
	Text string
	// Summary is the warning shown before the text; Sensitive with one.
	Summary   string
	Sensitive bool
	// ImageDescriptions describe its pictures, which the site shows
	// through itself by their index.
	ImageDescriptions []string
	// Videos are shown as their covers, which the site serves by their
	// index too.
	Videos      []FediverseVideo
	PublishedAt time.Time
	// UpdatedAt is zero for a post never edited.
	UpdatedAt time.Time
	// Likes are how many here like it, Liked whether the reader does:
	// its server does not learn of them.
	Likes int
	Liked bool
	// Comments are how many comments people here wrote under it.
	Comments int
	// Link is the card of the page it links to; nil for none.
	Link *FediverseLink
}

// FediverseLink is the card of a page a post of another server links
// to, as its server told it; its picture the site serves by the post.
type FediverseLink struct {
	URL         string
	Site        string
	Title       string
	Description string
	HasImage    bool
}

// FediverseVideo is a video of a post of another server.
type FediverseVideo struct {
	Description string
	// Blurhash is a blurred cover, drawn until the cover is read.
	Blurhash string
	// Width and Height are zero when the server did not tell.
	Width    int
	Height   int
	Duration time.Duration
}

// Picture is a picture of another server, read for the site to show.
type Picture struct {
	Body        []byte
	ContentType string
}

// FediverseComment is what a person here wrote under a post of another
// server; it stays here.
type FediverseComment struct {
	ID        string
	PostID    string
	AuthorID  string
	Text      string
	CreatedAt time.Time
}

// FediverseCommentPage is one screen of the comments of a post of another
// server, oldest first; Total counts them all.
type FediverseCommentPage struct {
	Comments   []FediverseComment
	NextCursor string
	Total      int
}
