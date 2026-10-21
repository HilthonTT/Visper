package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIPResolver extracts the real client IP from a request. Forwarding
// headers are only honoured when the direct peer is a trusted proxy;
// otherwise any client could spoof X-Forwarded-For to dodge per-IP limits.
type ClientIPResolver struct {
	trusted []netip.Prefix
}

// NewClientIPResolver parses trusted proxy CIDRs or bare IPs
// (e.g. "10.0.0.0/8", "127.0.0.1").
func NewClientIPResolver(trustedProxies []string) (*ClientIPResolver, error) {
	res := &ClientIPResolver{}
	for _, s := range trustedProxies {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			addr, err := netip.ParseAddr(s)
			if err != nil {
				return nil, fmt.Errorf("invalid trusted proxy %q: %w", s, err)
			}
			res.trusted = append(res.trusted, netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: %w", s, err)
		}
		res.trusted = append(res.trusted, p.Masked())
	}
	return res, nil
}

func (c *ClientIPResolver) isTrusted(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, p := range c.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// TrustsPeer reports whether the direct peer of r is a trusted proxy.
func (c *ClientIPResolver) TrustsPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	return err == nil && c.isTrusted(peer)
}

// ClientIP returns the client IP for r. When the peer is trusted, it walks
// X-Forwarded-For from right to left and returns the first untrusted hop.
func (c *ClientIPResolver) ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !c.isTrusted(peer) {
		return host
	}

	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			// Malformed entry: stop trusting anything further left.
			break
		}
		if !c.isTrusted(addr) {
			return addr.Unmap().String()
		}
		peer = addr
	}
	return peer.Unmap().String()
}
