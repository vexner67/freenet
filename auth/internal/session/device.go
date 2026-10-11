package session

import (
	"net/netip"
	"strings"
	"unicode"
)

const maxAgentLen = 255

// Device is where a session is used from, as its owner sees it in the list
// of their sessions: the address and what the browser calls itself. Both
// are whatever the client said, so they describe a session and prove
// nothing; either may be unknown.
type Device struct {
	ip    string
	agent string
}

// NewDevice takes what is known of the client as it is: an address that is
// not one is unknown, and the browser's name is cut to one short line. A
// sign-in is never refused over them.
func NewDevice(ip, agent string) Device {
	d := Device{agent: oneLine(agent)}

	if addr, err := netip.ParseAddr(strings.TrimSpace(ip)); err == nil {
		d.ip = addr.Unmap().String()
	}

	return d
}

func (d Device) IP() string {
	return d.ip
}

func (d Device) Agent() string {
	return d.agent
}

// oneLine drops control characters and keeps the first maxAgentLen runes.
func oneLine(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}

		return r
	}, s))

	if runes := []rune(s); len(runes) > maxAgentLen {
		s = strings.TrimSpace(string(runes[:maxAgentLen]))
	}

	return s
}
