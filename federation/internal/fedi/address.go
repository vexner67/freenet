package fedi

import (
	"regexp"
	"strings"
)

// Address is how a person names an account of another server: by its
// handle, username and host, or by the https address of the account.
type Address struct {
	Username string
	Host     string
	// URL is the address of the account when it is named so.
	URL URL
	// Channel is the name of a channel of Telegram when one is named.
	Channel string
}

var handle = regexp.MustCompile(`^@?([A-Za-z0-9_.\-]{1,64})@([A-Za-z0-9.\-]{1,253}\.[A-Za-z]{2,63})$`)

// NewAddress reads "@anya@mastodon.social", "anya@mastodon.social" or
// "https://mastodon.social/users/anya", or a channel of Telegram:
// "t.me/name", "https://t.me/s/name" or "@name". An account of site
// itself is not one of another server.
func NewAddress(s string, site Site) (Address, error) {
	s = strings.TrimSpace(s)

	if name := channelAddress(s); name != "" {
		return Address{Channel: name}, nil
	}

	if m := handle.FindStringSubmatch(s); m != nil {
		host := strings.ToLower(m[2])
		if host == site.Domain() {
			return Address{}, ErrInvalidAddress
		}

		return Address{Username: m[1], Host: host}, nil
	}

	u, err := NewURL(s)
	if err != nil || u.Host() == site.Domain() {
		return Address{}, ErrInvalidAddress
	}

	return Address{URL: u.WithoutFragment()}, nil
}
