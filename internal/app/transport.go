package app

import (
	"context"
	"net"
	"net/http"
	"time"
)

const (
	// idleConnsPerHost keeps a page's parallel reads (a calendar page looks
	// up every pick at once) on open connections for the next page; Go's
	// default keeps 2.
	idleConnsPerHost = 16
	// dialTimeout and dialKeepAlive match Go's default transport.
	dialTimeout   = 30 * time.Second
	dialKeepAlive = 30 * time.Second
	// h2PingAfter is how long an HTTP/2 connection may stay silent before it
	// is pinged; one that misses h2PingTimeout is closed, so a dead path
	// costs one failed request instead of every request until restart.
	h2PingAfter   = 30 * time.Second
	h2PingTimeout = 15 * time.Second
)

// newTransport is the one transport every client shares.
func newTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = idleConnsPerHost
	transport.DialContext = preferIPv4(&net.Dialer{Timeout: dialTimeout, KeepAlive: dialKeepAlive})
	transport.HTTP2 = &http.HTTP2Config{SendPingTimeout: h2PingAfter, PingTimeout: h2PingTimeout}
	return transport
}

// preferIPv4 dials IPv4 first and falls back to the requested network. On
// the production host about one IPv6 TCP flow to Telegram in eight goes
// dead (at connect or midway; ping and smaller MSS are unaffected, so it is
// per-flow loss, not MTU), while IPv4 flows all work. Go's Happy Eyeballs
// only races the connect, so it cannot leave a flow that dies later.
func preferIPv4(d *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if conn, err := d.DialContext(ctx, "tcp4", addr); err == nil {
			return conn, nil
		}
		return d.DialContext(ctx, network, addr)
	}
}
