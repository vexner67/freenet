package rest

import (
	"encoding/xml"
	"net/http"
	"strings"
)

// sitePages are the pages a search engine is told of: open to anyone and
// the same for everyone. People and communities are found through links.
var sitePages = []string{"/", "/about", "/terms", "/help"}

// robotsRules keep search engines off the pages of one's own (settings,
// messages, news...), the admin panel and the internal search; "$" ends a path, so that a
// person whose nickname starts so ("/imogen") is still indexed. The API
// is not disallowed: a crawler renders the pages with it, and its
// answers say noindex themselves.
const robotsRules = `User-agent: *
Disallow: /edit$
Disallow: /*/edit$
Disallow: /*/create$
Disallow: /settings$
Disallow: /invite$
Disallow: /language$
Disallow: /settings/
Disallow: /feed$
Disallow: /feed/
Disallow: /im$
Disallow: /im/
Disallow: /notifications$
Disallow: /fave$
Disallow: /fave/
Disallow: /docs$
Disallow: /stickers$
Disallow: /casino$
Disallow: /mine$
Disallow: /business$
Disallow: /votes$
Disallow: /prime$
Disallow: /admin$
Disallow: /admin/
Disallow: /search$
Disallow: /search?
Disallow: /media/documents/
`

// robots serves /robots.txt with the sitemap at the site's own address.
func (a *api) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", cacheRevalidate)
	_, _ = w.Write([]byte(robotsRules + "\nSitemap: " + siteOrigin(r, a.trustForwardedFor) + "/sitemap.xml\n"))
}

type sitemapURL struct {
	Loc string `xml:"loc"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// sitemap serves /sitemap.xml: sitePages at the site's own address.
func (a *api) sitemap(w http.ResponseWriter, r *http.Request) {
	origin := siteOrigin(r, a.trustForwardedFor)
	set := sitemapURLSet{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9"}

	for _, page := range sitePages {
		set.URLs = append(set.URLs, sitemapURL{Loc: origin + page})
	}

	body, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", cacheRevalidate)
	_, _ = w.Write(append([]byte(xml.Header), body...))
}

// siteOrigin is the address the site was asked at: HTTPS behind the proxy
// that says so (Caddy sets X-Forwarded-Proto), with the host it was asked
// for (the proxy sends to the gateway the host it was asked for itself).
func siteOrigin(r *http.Request, trustProxy bool) string {
	scheme := "http"
	if r.TLS != nil || (trustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")) {
		scheme = "https"
	}

	return scheme + "://" + r.Host
}

// noIndex keeps search engines from listing what is no page: the answers
// of the API, which a crawler still reads to render the pages, and the
// documents people attach.
func noIndex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/media/documents/") {
			w.Header().Set("X-Robots-Tag", "noindex")
		}

		next.ServeHTTP(w, r)
	})
}
