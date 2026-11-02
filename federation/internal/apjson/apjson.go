// Package apjson is ActivityPub in JSON: the documents of the pages and
// posts here, the activities they send, and what other servers send and
// say of their actors, as far as federation reads them. JSON-LD is read
// as the plain JSON the servers write: names, not expanded IRIs.
package apjson

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
)

// ContentType is the type of ActivityPub documents.
const ContentType = "application/activity+json"

var contexts = []any{
	"https://www.w3.org/ns/activitystreams",
	"https://w3id.org/security/v1",
	map[string]any{
		"manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
		"sensitive":                 "as:sensitive",
	},
}

type key struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	PublicKeyPem string `json:"publicKeyPem"`
}

type image struct {
	Type      string `json:"type"`
	MediaType string `json:"mediaType"`
	URL       string `json:"url"`
}

type person struct {
	Context                   any               `json:"@context,omitempty"`
	ID                        string            `json:"id"`
	Type                      string            `json:"type"`
	PreferredUsername         string            `json:"preferredUsername"`
	Name                      string            `json:"name"`
	Summary                   string            `json:"summary"`
	URL                       string            `json:"url"`
	Inbox                     string            `json:"inbox"`
	Outbox                    string            `json:"outbox"`
	Followers                 string            `json:"followers"`
	Endpoints                 map[string]string `json:"endpoints"`
	PublicKey                 key               `json:"publicKey"`
	Icon                      *image            `json:"icon,omitempty"`
	ManuallyApprovesFollowers bool              `json:"manuallyApprovesFollowers"`
	Discoverable              bool              `json:"discoverable"`
	Published                 string            `json:"published,omitempty"`
}

type note struct {
	Context      any      `json:"@context,omitempty"`
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	URL          string   `json:"url"`
	AttributedTo string   `json:"attributedTo"`
	Content      string   `json:"content"`
	Published    string   `json:"published"`
	Updated      string   `json:"updated,omitempty"`
	Sensitive    bool     `json:"sensitive"`
	To           []string `json:"to"`
	CC           []string `json:"cc"`
	Attachment   []image  `json:"attachment"`
}

type activity struct {
	Context any      `json:"@context,omitempty"`
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Actor   string   `json:"actor"`
	To      []string `json:"to,omitempty"`
	CC      []string `json:"cc,omitempty"`
	Object  any      `json:"object"`
}

type collection struct {
	Context    any    `json:"@context"`
	ID         string `json:"id"`
	Type       string `json:"type"`
	TotalItems int    `json:"totalItems"`
}

// Person is the document of a page.
func Person(p fedi.Person) ([]byte, error) {
	doc := person{
		Context:           contexts,
		ID:                p.ID.String(),
		Type:              "Person",
		PreferredUsername: p.Nickname,
		Name:              p.Name,
		Summary:           p.Summary,
		URL:               p.Page.String(),
		Inbox:             p.Inbox.String(),
		Outbox:            p.Outbox.String(),
		Followers:         p.Followers.String(),
		Endpoints:         map[string]string{"sharedInbox": p.Shared.String()},
		PublicKey:         key{ID: p.KeyID.String(), Owner: p.ID.String(), PublicKeyPem: p.PublicKey},
		Discoverable:      true,
		Published:         timestamp(p.Published),
	}

	if p.Avatar != "" {
		doc.Icon = &image{Type: "Image", MediaType: "image/jpeg", URL: p.Avatar.String()}
	}

	return marshal(doc)
}

// Note is the document of a post.
func Note(n fedi.Note) ([]byte, error) {
	doc := toNote(n)
	doc.Context = contexts

	return marshal(doc)
}

func toNote(n fedi.Note) note {
	attachments := make([]image, len(n.Attachments))
	for i, u := range n.Attachments {
		attachments[i] = image{Type: "Document", MediaType: "image/jpeg", URL: u.String()}
	}

	return note{
		ID:           n.ID.String(),
		Type:         "Note",
		URL:          n.Page.String(),
		AttributedTo: n.AttributedTo.String(),
		Content:      n.Content,
		Published:    timestamp(n.Published),
		Updated:      timestamp(n.Updated),
		Sensitive:    n.Sensitive,
		To:           n.To,
		CC:           n.CC,
		Attachment:   attachments,
	}
}

// Collection is an ordered collection told only by its size: who follows
// a page and what it posted are not listed.
func Collection(id fedi.URL, total int) ([]byte, error) {
	return marshal(collection{Context: contexts[0], ID: id.String(), Type: "OrderedCollection", TotalItems: total})
}

// Activity is an activity a page sends.
func Activity(a fedi.Outgoing) ([]byte, error) {
	doc := activity{
		Context: contexts,
		ID:      a.ID.String(),
		Type:    a.Type,
		Actor:   a.Actor.String(),
		To:      a.To,
		CC:      a.CC,
	}

	switch {
	case a.Note != nil:
		doc.Object = toNote(*a.Note)
	case a.Tombstone != "":
		doc.Object = map[string]string{"id": a.Tombstone.String(), "type": "Tombstone"}
	case a.Follow != nil:
		doc.Object = map[string]string{"id": a.Follow.ID, "type": a.Follow.Type, "actor": a.Follow.Actor.String(), "object": a.Follow.Object}
	default:
		doc.Object = a.Person.String()
	}

	return marshal(doc)
}

type link struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href"`
}

type finger struct {
	Subject string   `json:"subject"`
	Aliases []string `json:"aliases"`
	Links   []link   `json:"links"`
}

// Finger is the WebFinger answer for a page: where its document is.
func Finger(p fedi.Person, domain string) ([]byte, error) {
	return marshal(finger{
		Subject: "acct:" + p.Nickname + "@" + domain,
		Aliases: []string{p.ID.String(), p.Page.String()},
		Links: []link{
			{Rel: "self", Type: ContentType, Href: p.ID.String()},
			{Rel: "http://webfinger.net/rel/profile-page", Type: "text/html", Href: p.Page.String()},
		},
	})
}

func timestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	return t.UTC().Format(time.RFC3339)
}

// marshal writes JSON without escaping <, > and &, which the HTML of
// notes is full of.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer

	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return nil, errs.Wrap("enc.Encode", err)
	}

	return bytes.TrimRight(b.Bytes(), "\n"), nil
}
