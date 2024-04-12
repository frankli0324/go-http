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

func defaultDial(ctx context.Context, r *PreparedRequest) (io.ReadWriteCloser, error) {
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
				c := tls.Client(conn, &tls.Config{
					ServerName: r.U.Hostname(),
				})
				return c, c.HandshakeContext(ctx)
			}
			return conn, nil
		},
	})
}
