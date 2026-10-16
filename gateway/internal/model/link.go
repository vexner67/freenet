package model

import (
	"regexp"
	"strings"
)

// linkPattern finds an address in a text as the website does (labels.js
// linkParts): http or https, up to a space or a character no address has.
var linkPattern = regexp.MustCompile(`\bhttps?://[^\s<>"«»\[\]]+`)

// FirstLink is the first web address text says, without the punctuation
// that ends a sentence after it; "" for none. A post shows the card of
// that page.
func FirstLink(text string) string {
	link := linkPattern.FindString(text)
	if link == "" {
		return ""
	}

	return trimLink(link)
}

// trimLink drops what ends a sentence rather than an address: ".", ",",
// "!" and the like, and ")" that closes no "(" of the address.
func trimLink(link string) string {
	for {
		trimmed := strings.TrimRight(link, ".,;:!?'\"’”…")
		if strings.HasSuffix(trimmed, ")") && strings.Count(trimmed, "(") < strings.Count(trimmed, ")") {
			trimmed = trimmed[:len(trimmed)-1]
		}

		if trimmed == link {
			break
		}
		link = trimmed
	}

	// Nothing is left of an address that was its scheme alone.
	if strings.HasSuffix(link, "://") {
		return ""
	}

	return link
}
