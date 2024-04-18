//go:build !go1.18
// +build !go1.18

package dialer

import (
	"crypto/tls"
	"net"
)

// a helper type to make tls.Conn implement syscall.Conn interface
type sysTLSConn struct {
	*tls.Conn
	underlying net.Conn
}

func (c sysTLSConn) NetConn() net.Conn {
	return c.underlying
}

// polyfill for go1.17 *[tls.Conn.NetConn]
func wrapTLS(c *tls.Conn, raw net.Conn) net.Conn {
	return sysTLSConn{c, raw}
}
