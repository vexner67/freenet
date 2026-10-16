// Package preview reads what a web page says of itself — its title,
// description, site and picture — for the card of a post that links to it.
//
// The page is anyone's choice, so the reader never goes where the gateway
// alone could: it connects only to public addresses (checked on the
// address it actually dials, after DNS, so a name that resolves inside
// does not pass), on ports 80 and 443, without a proxy, through at most a
// few redirects, and reads a bounded number of bytes in a bounded time.
package preview

import (
	"context"
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
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	"github.com/vexner67/freenet/gateway/internal/model"
)

const (
	maxPageBytes  = 512 << 10
	maxImageBytes = 5 << 20
	maxRedirects  = 3

	maxTitle       = 256
	maxDescription = 512
	maxSite        = 128

	userAgent = "Mozilla/5.0 (compatible; FreenetBot/1.0; link preview)"
)

var (
	// ErrNoPreview: the page tells nothing to show (not HTML, no title).
	ErrNoPreview = errors.New("the page has no preview")
	// ErrForbidden: the address is not one the reader goes to.
	ErrForbidden = errors.New("the address is not allowed")
)

// Reader reads the cards of pages; its zero value is not usable, see New.
type Reader struct {
	client  *http.Client
	timeout time.Duration
	allowed func(netip.AddrPort) bool
}

// New is a Reader that spends at most timeout on a page and its picture.
func New(timeout time.Duration) *Reader {
	return newReader(timeout, Public)
}

// newReader dials only the addresses allowed tells; tests allow loopback.
func newReader(timeout time.Duration, allowed func(netip.AddrPort) bool) *Reader {
	r := &Reader{timeout: timeout, allowed: allowed}

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

	r.client = &http.Client{
		Transport: &http.Transport{
			Proxy:                  nil,
			DialContext:            dialer.DialContext,
			TLSHandshakeTimeout:    timeout,
			ResponseHeaderTimeout:  timeout,
			MaxResponseHeaderBytes: 64 << 10,
			IdleConnTimeout:        30 * time.Second,
			MaxIdleConns:           16,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			return checkURL(req.URL)
		},
	}

	return r
}

// Public allows the addresses of the internet on the ports of the web:
// not loopback, private, link-local, shared (CGNAT), documentation or
// other special ranges, in IPv4 or hidden inside IPv6.
func Public(ap netip.AddrPort) bool {
	if ap.Port() != 80 && ap.Port() != 443 {
		return false
	}

	return publicAddr(ap.Addr())
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

// checkURL lets through web addresses only: no other schemes and no
// user names, which could hide where a link really goes.
func checkURL(u *url.URL) error {
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ErrForbidden
	}

	return nil
}

// Read is the card of the page at rawURL; its picture is left out when
// it cannot be read. URL is rawURL itself, where the card leads.
func (r *Reader) Read(ctx context.Context, rawURL string) (model.LinkPreview, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	u, err := url.Parse(rawURL)
	if err != nil {
		return model.LinkPreview{}, fmt.Errorf("url.Parse: %w", err)
	}

	if err = checkURL(u); err != nil {
		return model.LinkPreview{}, err
	}

	page, final, err := r.page(ctx, u)
	if err != nil {
		return model.LinkPreview{}, err
	}

	if page.title == "" {
		return model.LinkPreview{}, ErrNoPreview
	}

	p := model.LinkPreview{
		URL:         rawURL,
		Title:       cut(page.title, maxTitle),
		Description: cut(page.description, maxDescription),
		Site:        cut(page.site, maxSite),
	}
	if p.Site == "" {
		p.Site = cut(strings.TrimPrefix(final.Hostname(), "www."), maxSite)
	}

	if page.image != "" {
		if image, err := final.Parse(page.image); err == nil && checkURL(image) == nil {
			// The card goes on without its picture.
			p.Image, _ = r.image(ctx, image)
		}
	}

	return p, nil
}

// page reads the head of the page at u; final is where it was found,
// after redirects, against which its links resolve.
func (r *Reader) page(ctx context.Context, u *url.URL) (meta, *url.URL, error) {
	resp, err := r.get(ctx, u, "text/html,application/xhtml+xml")
	if err != nil {
		return meta{}, nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // read only

	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
		return meta{}, nil, ErrNoPreview
	}

	body, err := charset.NewReader(io.LimitReader(resp.Body, maxPageBytes), resp.Header.Get("Content-Type"))
	if err != nil {
		return meta{}, nil, fmt.Errorf("charset.NewReader: %w", err)
	}

	return parse(body), resp.Request.URL, nil
}

// image is the picture at u, if it is one and not too big.
func (r *Reader) image(ctx context.Context, u *url.URL) ([]byte, error) {
	resp, err := r.get(ctx, u, "image/*")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // read only

	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") || resp.ContentLength > maxImageBytes {
		return nil, ErrNoPreview
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("io.ReadAll: %w", err)
	}

	if len(data) > maxImageBytes {
		return nil, ErrNoPreview
	}

	return data, nil
}

func (r *Reader) get(ctx context.Context, u *url.URL, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("http.NewRequestWithContext: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "ru,en;q=0.8")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("r.client.Do: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("status %d: %w", resp.StatusCode, ErrNoPreview)
	}

	return resp, nil
}

// meta is what a page says of itself, by the best of its tags.
type meta struct {
	title, description, site, image string
}

// parse reads the tags of the head: Open Graph first, then Twitter's,
// then the plain <title> and description.
func parse(body io.Reader) meta {
	found := map[string]string{}
	var title string

	z := html.NewTokenizer(body)
	for {
		switch z.Next() {
		case html.ErrorToken:
			return pick(found, title)
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()

			switch t.Data {
			case "body":
				// What a page says of itself is in its head.
				return pick(found, title)
			case "title":
				if title == "" && z.Next() == html.TextToken {
					title = string(z.Text())
				}
			case "meta":
				key, content := metaTag(t)
				if _, ok := found[key]; key != "" && !ok {
					found[key] = content
				}
			}
		}
	}
}

// metaTag is the name (or property) of a <meta> and what it says.
func metaTag(t html.Token) (key, content string) {
	for _, a := range t.Attr {
		switch a.Key {
		case "property", "name":
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(a.Val))
			}
		case "content":
			content = a.Val
		}
	}

	return key, content
}

func pick(found map[string]string, title string) meta {
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := clean(found[k]); v != "" {
				return v
			}
		}
		return ""
	}

	m := meta{
		title:       first("og:title", "twitter:title"),
		description: first("og:description", "twitter:description", "description"),
		site:        first("og:site_name"),
		image:       strings.TrimSpace(first("og:image", "og:image:url", "og:image:secure_url", "twitter:image", "twitter:image:src")),
	}
	if m.title == "" {
		m.title = clean(title)
	}

	return m
}

// clean is s in one line, its spaces collapsed, without invalid UTF-8.
func clean(s string) string {
	return strings.Join(strings.Fields(strings.ToValidUTF8(s, "")), " ")
}

// cut is s of at most n characters, an ellipsis ending what was cut.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}

	return strings.TrimSpace(string([]rune(s)[:n-1])) + "…"
}
