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

func (d *CoreDialer) Clone() Dialer {
	return &CoreDialer{
		TLSConfig: d.TLSConfig,
	}
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
			if c, err := tryDialH2(hp); err == nil {
				return c, err
			}
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
				if err := c.HandshakeContext(ctx); err != nil {
					return nil, err
				}
				if c.ConnectionState().NegotiatedProtocol == "h2" {
					return negotiateNewH2(hp, c) // must succeed since already negotiated h2
				}
				return c, nil
			}
			return conn, nil
		},
	})
}
