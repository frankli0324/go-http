package internal

import (
	"crypto/tls"
	"io"
	"net"
	"time"

	"github.com/frankli0324/go-http/internal/dialer"
	"github.com/frankli0324/go-http/internal/transport/h2c"
	"github.com/frankli0324/go-http/utils/netpool"
)

var defaultDialer = &dialer.CoreDialer{
	TLSConfig: &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
	},
	ProxyConfig: &dialer.ProxyConfig{
		TLSConfig:      &tls.Config{}, // don't want h2
		ResolveLocally: false,
	},
	ConnPool: netpool.NewGroup(100, 0, 90*time.Second),
}

func getRawConn(c io.ReadWriteCloser) net.Conn {
	if conn, ok := c.(interface{ Raw() net.Conn }); ok {
		return conn.Raw()
	}
	return nil
}

func getTLSConn(c io.ReadWriteCloser) *tls.Conn {
	raw := getRawConn(c)
	if tls, ok := raw.(*tls.Conn); ok {
		return tls
	}
	if str, ok := raw.(*h2c.Stream); ok {
		if tls, ok := str.Conn.(*tls.Conn); ok {
			return tls
		}
	}
	return nil
}
