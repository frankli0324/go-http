//go:build go1.18
// +build go1.18

package dialer

import (
	"crypto/tls"
	"net"
)

func wrapTLS(c *tls.Conn, _ net.Conn) net.Conn {
	return c
}
