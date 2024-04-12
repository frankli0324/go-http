package internal

import (
	"context"
	"crypto/tls"
	"io"
	"net"

	"github.com/frankli0324/go-http/netpool"
)

var pool = netpool.NewGroup(100, 80)
var schemes = map[string]string{
	"http": ":80", "https": ":443",
}

var zeroDialer net.Dialer

type CoreDialer struct {
	TLSConfig *tls.Config // the config to use
}

var defaultDialer = &CoreDialer{
	TLSConfig: &tls.Config{
		NextProtos: []string{"h2"},
	},
}

func (d *CoreDialer) Unwrap() Dialer {
	return nil
}

func (d *CoreDialer) Dial(ctx context.Context, r *PreparedRequest) (io.ReadWriteCloser, error) {
	hp := r.U.Host
	if r.U.Port() == "" {
		hp = r.U.Hostname() + schemes[r.U.Scheme]
	}
	return pool.Connect(ctx, netpool.ConnRequest{
		Key: hp, Dial: func(ctx context.Context) (net.Conn, error) {
			conn, err := zeroDialer.DialContext(ctx, "tcp", hp)
			if err != nil {
				return nil, err
			}
			if r.U.Scheme == "https" {
				config := d.TLSConfig.Clone()
				if config == nil {
					config = &tls.Config{}
				}
				config.ServerName = r.U.Hostname()
				c := tls.Client(conn, config)
				return c, c.HandshakeContext(ctx)
			}
			return conn, nil
		},
	})
}

func getTLSConn(c io.ReadWriteCloser) *tls.Conn {
	if conn, ok := c.(interface{ Raw() net.Conn }); ok {
		if tls, ok := conn.Raw().(*tls.Conn); ok {
			return tls
		}
	}
	return nil
}
