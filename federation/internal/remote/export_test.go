package remote

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
)

// NewForTest is a client of srv, a TLS server on loopback.
func NewForTest(site fedi.Site, srv *httptest.Server) *Client {
	c := newClient(site, 5*time.Second, func(netip.AddrPort) bool { return true })
	c.http.Transport.(*http.Transport).TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig

	return c
}
