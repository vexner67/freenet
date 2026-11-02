// Package remote talks to other servers of the Fediverse over HTTPS: it
// reads the documents of their actors and posts activities to their
// inboxes. It dials only the internet: an address on another server may
// name any host, and never one inside the network the service is in.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/vexner67/freenet/federation/internal/apjson"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/httpsig"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

const (
	userAgent       = "Freenet (+https://github.com/vexner67/freenet)"
	maxDocumentSize = 1 << 20
	maxRedirects    = 3
	accept          = `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams"`
)

// ErrForbidden: the address is not one the client goes to.
var ErrForbidden = errors.New("the address is not allowed")

// StatusError is an answer of a remote server other than success.
type StatusError struct {
	Code int
}

func (e StatusError) Error() string {
	return fmt.Sprintf("remote status %d", e.Code)
}

// Client is the service.Resolver and posts activities.
type Client struct {
	http *http.Client
	site fedi.Site
	now  func() time.Time
}

// New is a client of the pages of site that spends at most timeout on a
// request and dials only public addresses.
func New(site fedi.Site, timeout time.Duration) *Client {
	return newClient(site, timeout, Public)
}

// newClient dials only what allowed lets through; tests allow loopback.
func newClient(site fedi.Site, timeout time.Duration, allowed func(netip.AddrPort) bool) *Client {
	dialer := &net.Dialer{
		Timeout: timeout,
		// Control sees the address being dialed, after DNS: the only
		// check a name resolving to an inside address cannot pass.
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || !allowed(ap) {
				return ErrForbidden
			}
			return nil
		},
	}

	return &Client{
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy:                  nil,
				DialContext:            dialer.DialContext,
				TLSHandshakeTimeout:    timeout,
				ResponseHeaderTimeout:  timeout,
				MaxResponseHeaderBytes: 64 << 10,
				IdleConnTimeout:        90 * time.Second,
				MaxIdleConnsPerHost:    4,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects || req.URL.Scheme != "https" {
					return ErrForbidden
				}
				return nil
			},
		},
		site: site,
		now:  time.Now,
	}
}

// Actor reads the document of an actor, signing the request as signer
// when there is one.
func (c *Client) Actor(ctx context.Context, id fedi.URL, signer *fedi.Account) (fedi.RemoteDoc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, id.String(), nil)
	if err != nil {
		return fedi.RemoteDoc{}, errs.Wrap("http.NewRequestWithContext", err)
	}

	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", userAgent)

	if signer != nil {
		if err = httpsig.Sign(req, nil, c.site.Key(signer.UserID()), signer.Keys().Private, c.now()); err != nil {
			return fedi.RemoteDoc{}, errs.Wrap("httpsig.Sign", err)
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fedi.RemoteDoc{}, errs.Wrap("c.http.Do", err)
	}
	defer func() { _ = resp.Body.Close() }() // a read body has nothing more to tell

	switch {
	case resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound:
		return fedi.RemoteDoc{}, fedi.ErrGone
	case resp.StatusCode != http.StatusOK:
		return fedi.RemoteDoc{}, StatusError{Code: resp.StatusCode}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDocumentSize))
	if err != nil {
		return fedi.RemoteDoc{}, errs.Wrap("io.ReadAll", err)
	}

	doc, err := apjson.Actor(body)
	if err != nil {
		return fedi.RemoteDoc{}, errs.Wrap("apjson.Actor", err)
	}

	return doc, nil
}

// Post sends an activity to an inbox, signed with the key of signer.
func (c *Client) Post(ctx context.Context, inbox fedi.URL, body []byte, signer *fedi.Account) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inbox.String(), bytes.NewReader(body))
	if err != nil {
		return errs.Wrap("http.NewRequestWithContext", err)
	}

	req.Header.Set("Content-Type", apjson.ContentType)
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", userAgent)

	if err = httpsig.Sign(req, body, c.site.Key(signer.UserID()), signer.Keys().Private, c.now()); err != nil {
		return errs.Wrap("httpsig.Sign", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return errs.Wrap("c.http.Do", err)
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDocumentSize)) // drained to reuse the connection
	_ = resp.Body.Close()                                                  // a drained body has nothing more to tell

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return StatusError{Code: resp.StatusCode}
	}

	return nil
}

// Public allows the addresses of the internet on the port of HTTPS: not
// loopback, private, link-local, shared (CGNAT), documentation or other
// special ranges, in IPv4 or hidden inside IPv6.
func Public(ap netip.AddrPort) bool {
	return ap.Port() == 443 && publicAddr(ap.Addr())
}

// special are ranges IsGlobalUnicast lets through but no public site has.
var special = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	// IPv6 ways to reach IPv4 addresses (NAT64, 6to4, Teredo) and the
	// documentation range.
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func publicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()

	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}

	for _, p := range special {
		if p.Contains(ip) {
			return false
		}
	}

	return true
}

// maxPictureSize is the largest picture of another server the site shows.
const maxPictureSize = 8 << 20

// pictures are the types of pictures the site shows: browsers show them
// all, and none of them runs a script, as SVG can.
var pictures = map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}

// VideoHeadSize is how much of a video the site reads to cut its first
// frame: servers put what tells how to play it first ("faststart"), and
// the first frame soon after.
const VideoHeadSize = 4 << 20

// VideoHead is the beginning of a video of another server, at most
// VideoHeadSize; one that is no video is fedi.ErrNotFound.
func (c *Client) VideoHead(ctx context.Context, u fedi.URL) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errs.Wrap("http.NewRequestWithContext", err)
	}

	req.Header.Set("Accept", "video/*")
	req.Header.Set("User-Agent", userAgent)
	// A server that does not serve a part sends the whole: only the head
	// of it is read.
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", VideoHeadSize-1))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errs.Wrap("c.http.Do", err)
	}
	defer func() { _ = resp.Body.Close() }() // what is not read is not needed

	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent || !strings.HasPrefix(contentType, "video/") {
		return nil, fedi.ErrNotFound
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, VideoHeadSize))
	return body, errs.Wrap("io.ReadAll", err)
}

type fingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type"`
	Href string `json:"href"`
}

// Finger finds the actor of @username@host by WebFinger.
func (c *Client) Finger(ctx context.Context, username, host string) (fedi.URL, error) {
	u := url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/.well-known/webfinger",
		RawQuery: url.Values{"resource": {"acct:" + username + "@" + host}}.Encode(),
	}

	body, err := c.get(ctx, u.String(), "application/jrd+json, application/json", maxDocumentSize)
	if err != nil {
		return "", err
	}

	var doc struct {
		Links []fingerLink `json:"links"`
	}
	if err = json.Unmarshal(body, &doc); err != nil {
		return "", errs.Wrap("json.Unmarshal", err)
	}

	for _, l := range doc.Links {
		if l.Rel != "self" || !strings.Contains(l.Type, "activity+json") && !strings.Contains(l.Type, "ld+json") {
			continue
		}

		id, err := fedi.NewURL(l.Href)
		if err != nil {
			return "", errs.Wrap("fedi.NewURL", err)
		}
		return id, nil
	}

	return "", fedi.ErrNotFound
}

// Fetch reads a picture; anything else is fedi.ErrNotFound.
func (c *Client) Fetch(ctx context.Context, u fedi.URL) (service.Media, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return service.Media{}, errs.Wrap("http.NewRequestWithContext", err)
	}

	req.Header.Set("Accept", "image/webp, image/png, image/jpeg, image/gif")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return service.Media{}, errs.Wrap("c.http.Do", err)
	}
	defer func() { _ = resp.Body.Close() }() // a read body has nothing more to tell

	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || !pictures[contentType] || resp.ContentLength > maxPictureSize {
		return service.Media{}, fedi.ErrNotFound
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPictureSize+1))
	if err != nil {
		return service.Media{}, errs.Wrap("io.ReadAll", err)
	}

	if len(body) > maxPictureSize {
		return service.Media{}, fedi.ErrNotFound
	}

	return service.Media{Body: body, ContentType: contentType}, nil
}

// get reads a document of at most limit bytes.
func (c *Client) get(ctx context.Context, u, accept string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errs.Wrap("http.NewRequestWithContext", err)
	}

	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errs.Wrap("c.http.Do", err)
	}
	defer func() { _ = resp.Body.Close() }() // a read body has nothing more to tell

	switch {
	case resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound:
		return nil, fedi.ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return nil, StatusError{Code: resp.StatusCode}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return body, errs.Wrap("io.ReadAll", err)
}
