package preview

import (
	"net/netip"
	"time"
)

// NewAllowing is a Reader that dials what allowed tells, as tests need to
// reach their own loopback servers.
func NewAllowing(timeout time.Duration, allowed func(netip.AddrPort) bool) *Reader {
	return newReader(timeout, allowed)
}
