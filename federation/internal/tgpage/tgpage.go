// Package tgpage reads the page of a public channel of Telegram,
// t.me/s/<channel>: its title, user name and photo, and its latest posts
// as notes of its account, with their pictures and the covers of their
// videos.
package tgpage

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
	"golang.org/x/net/html"
)

var (
	backgroundImage = regexp.MustCompile(`background-image:\s*url\(['"]?([^'")]+)['"]?\)`)
	styleWidth      = regexp.MustCompile(`(?:^|;)\s*width:\s*(\d+(?:\.\d+)?)px`)
	stylePadding    = regexp.MustCompile(`padding-top:\s*(\d+(?:\.\d+)?)%`)
	postNumber      = regexp.MustCompile(`^[A-Za-z0-9_]+/(\d{1,12})$`)
)

// Parse reads the page of the channel name. A page with no channel on it
// (Telegram shows another page for a name of nobody, a person or a
// private channel) is fedi.ErrNotFound.
func Parse(page []byte, name string) (fedi.ChannelDoc, error) {
	root, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return fedi.ChannelDoc{}, errs.Wrap("html.Parse", err)
	}

	info := find(root, withClass("tgme_channel_info"))
	if info == nil {
		return fedi.ChannelDoc{}, fedi.ErrNotFound
	}

	doc := fedi.ChannelDoc{
		Title:    strings.TrimSpace(text(find(info, withClass("tgme_channel_info_header_title")))),
		Username: strings.TrimPrefix(strings.TrimSpace(text(find(info, withClass("tgme_channel_info_header_username")))), "@"),
	}

	if photo := find(info, withClass("tgme_page_photo_image")); photo != nil {
		doc.Photo = attr(find(photo, isTag("img")), "src")
	}

	actor := fedi.ChannelURL(name)
	for _, m := range findAll(root, withClass("tgme_widget_message")) {
		if note, ok := message(m, actor); ok {
			doc.Posts = append(doc.Posts, note)
		}
	}

	return doc, nil
}

// message is a post of the channel as a note of its account; false for
// one the page tells too little about.
func message(m *html.Node, actor fedi.URL) (fedi.NoteDoc, bool) {
	number := postNumber.FindStringSubmatch(attr(m, "data-post"))
	if number == nil {
		return fedi.NoteDoc{}, false
	}

	published, err := time.Parse(time.RFC3339, attr(find(find(m, withClass("tgme_widget_message_date")), isTag("time")), "datetime"))
	if err != nil {
		return fedi.NoteDoc{}, false
	}

	id := actor.String() + "/" + number[1]
	note := fedi.NoteDoc{
		ID:           id,
		AttributedTo: actor.String(),
		URL:          id,
		Public:       true,
		Published:    published,
	}

	// The text of the post, not that of the post it replies to.
	if body := find(m, withClass("js-message_text")); body != nil {
		note.Text = plain(body)
	}

	for _, p := range findAll(m, withClass("tgme_widget_message_photo_wrap")) {
		if u := background(p); u != "" {
			note.Attachments = append(note.Attachments, fedi.AttachmentDoc{URL: u, MediaType: "image/jpeg"})
		}
	}

	for _, v := range findAll(m, withClass("tgme_widget_message_video_player")) {
		note.Attachments = append(note.Attachments, video(v, id))
	}

	if preview := find(m, withClass("tgme_widget_message_link_preview")); preview != nil {
		note.Link = card(preview)
	}

	return note, true
}

// card is the card Telegram made of the page a post links to.
func card(n *html.Node) fedi.LinkDoc {
	image := find(n, withClass("link_preview_image"))
	if image == nil {
		image = find(n, withClass("link_preview_right_image"))
	}

	return fedi.LinkDoc{
		URL:         attr(n, "href"),
		Site:        tidy(text(find(n, withClass("link_preview_site_name")))),
		Title:       tidy(text(find(n, withClass("link_preview_title")))),
		Description: tidy(text(find(n, withClass("link_preview_description")))),
		Image:       background(image),
	}
}

// video is a video of a post: its cover, its shape and how long it
// plays. A video too big for the page has no address there: the post
// stands for it, which no frame is cut of.
func video(v *html.Node, post string) fedi.AttachmentDoc {
	a := fedi.AttachmentDoc{
		URL:       attr(find(v, isTag("video")), "src"),
		MediaType: "video/mp4",
		Cover:     background(find(v, withClass("tgme_widget_message_video_thumb"))),
		Duration:  duration(text(find(v, withClass("message_video_duration")))),
	}
	if a.URL == "" {
		a.URL = post
	}

	style := attr(find(v, withClass("tgme_widget_message_video_wrap")), "style")
	width, height := number(styleWidth, style), number(stylePadding, style)
	if width > 0 && height > 0 {
		a.Width, a.Height = int(width), int(width*height/100)
	}

	return a
}

// duration reads "0:42", "12:05" or "1:02:03".
func duration(s string) time.Duration {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}

	var d time.Duration
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0
		}
		d = d*60 + time.Duration(n)
	}

	return d * time.Second
}

func number(re *regexp.Regexp, s string) float64 {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0
	}

	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}

	return n
}

// background is the address of the picture n is drawn with.
func background(n *html.Node) string {
	if m := backgroundImage.FindStringSubmatch(attr(n, "style")); m != nil {
		return m[1]
	}

	return ""
}

// plain is the text of a post as plain text: lines kept, links as the
// words they are written with, emoji as their characters.
func plain(n *html.Node) string {
	var b strings.Builder

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			b.WriteString(n.Data)
		case n.Type == html.ElementNode && n.Data == "br":
			b.WriteString("\n")
		case n.Type == html.ElementNode && n.Data == "a":
			b.WriteString(linkText(n))
			return
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	return tidy(b.String())
}

// maxLinkWords bounds the words of a link the site shows as the link.
const maxLinkWords = 300

// linkText is a link of a post as plain text, its address there for the
// site to make it a link again: the address alone when the link shows
// it, the words of the link as VK's wiki links write them otherwise
// ("[https://…|за день]"), which the site shows as the words leading to
// the address. Words that cannot be put so go before the address in
// brackets ("за день (https://…)"). Mentions, hashtags and links of
// Telegram's own stay words.
func linkText(a *html.Node) string {
	words := text(a)
	href := attr(a, "href")

	if !strings.HasPrefix(href, "https://") && !strings.HasPrefix(href, "http://") ||
		strings.HasPrefix(strings.TrimSpace(words), "@") || strings.HasPrefix(strings.TrimSpace(words), "#") {
		return words
	}

	// Words with no dot are no address, though "me" begins "meduza.io".
	shown := bare(strings.TrimRight(strings.TrimSpace(words), "…."))
	if shown == "" || strings.Contains(shown, ".") && strings.HasPrefix(bare(href), shown) {
		return href
	}

	named := strings.TrimSpace(words)
	if strings.ContainsAny(named, "[]|\n") || strings.ContainsAny(href, "[]| \t\n") || utf8.RuneCountInString(named) > maxLinkWords {
		return words + " (" + href + ")"
	}

	return "[" + href + "|" + named + "]"
}

// bare is an address without its scheme and "www.".
func bare(s string) string {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	return strings.TrimPrefix(s, "www.")
}

// tidy drops the spaces at the ends of lines and more than one empty
// line in a row.
func tidy(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, " ", " "), "\n")
	out := make([]string, 0, len(lines))
	empty := 0

	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			empty++
			if empty > 1 {
				continue
			}
		} else {
			empty = 0
		}
		out = append(out, l)
	}

	return strings.TrimSpace(strings.Join(out, "\n"))
}

func text(n *html.Node) string {
	if n == nil {
		return ""
	}

	var b strings.Builder

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	return b.String()
}

func attr(n *html.Node, name string) string {
	if n == nil {
		return ""
	}

	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}

	return ""
}

func withClass(class string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && hasClass(n, class)
	}
}

func isTag(tag string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == tag
	}
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}

	return false
}

// find is the first node under n, n itself excluded, that ok holds for.
func find(n *html.Node, ok func(*html.Node) bool) *html.Node {
	if n == nil {
		return nil
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if ok(c) {
			return c
		}
		if found := find(c, ok); found != nil {
			return found
		}
	}

	return nil
}

// findAll are the nodes under n that ok holds for, not looking inside
// those found.
func findAll(n *html.Node, ok func(*html.Node) bool) []*html.Node {
	var out []*html.Node

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if ok(c) {
			out = append(out, c)
			continue
		}
		out = append(out, findAll(c, ok)...)
	}

	return out
}
