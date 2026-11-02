package fedi

import (
	"regexp"
	"strings"
	"time"
)

// TelegramHost is where the public channels of Telegram are. The site
// reads them as accounts with no server of the Fediverse behind them:
// their pages there, t.me/s/<channel>, are what it follows.
const TelegramHost = "t.me"

// channelName is the user name of a public channel of Telegram.
const channelName = `([A-Za-z][A-Za-z0-9_]{3,31})`

var (
	channelLink   = regexp.MustCompile(`^(?i:https?://)?(?i:www\.)?(?i:t\.me|telegram\.me)/(?:s/)?` + channelName + `/?$`)
	channelHandle = regexp.MustCompile(`^@` + channelName + `$`)
)

// channelAddress is the name of the channel s names: "t.me/name",
// "https://t.me/s/name" or "@name"; "" for none.
func channelAddress(s string) string {
	if m := channelLink.FindStringSubmatch(s); m != nil {
		return strings.ToLower(m[1])
	}

	if m := channelHandle.FindStringSubmatch(s); m != nil {
		return strings.ToLower(m[1])
	}

	return ""
}

// ChannelURL is the address of the account of a channel: the one people
// open it at.
func ChannelURL(name string) URL {
	return URL("https://" + TelegramHost + "/" + strings.ToLower(name))
}

// IsChannel: u is the account of a channel of Telegram, or a post of one.
func IsChannel(u URL) bool {
	return u.Host() == TelegramHost
}

// ChannelName is the name of the channel of the account u.
func ChannelName(u URL) string {
	return strings.TrimPrefix(u.String(), "https://"+TelegramHost+"/")
}

// ChannelDoc is what the page of a channel tells: its title, user name and
// photo, and its latest posts as notes of its account.
type ChannelDoc struct {
	Title    string
	Username string
	Photo    string
	Posts    []NoteDoc
}

// NewChannel is the account of the channel name as its page tells it; a
// page of another channel is not believed. A channel has no inbox and no
// key: the site reads its page, nobody sends to it.
func NewChannel(name string, doc ChannelDoc, now time.Time) (*Remote, error) {
	if !strings.EqualFold(doc.Username, name) || channelAddress("@"+name) == "" {
		return nil, ErrInvalidActivity
	}

	id := ChannelURL(name)
	p := newProfile(RemoteDoc{Name: doc.Title, Username: doc.Username, Icon: doc.Photo, Page: id.String()})

	return &Remote{id: id, profile: p, fetchedAt: now.UTC()}, nil
}
