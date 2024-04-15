package internal

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/url"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/netpool"
)

var pool = netpool.NewGroup(100, 80)

var schemes = map[string]string{
	"http": ":80", "https": ":443", "socks": ":1080",
}

var zeroDialer net.Dialer

type CoreDialer struct {
	TLSConfig      *tls.Config // the config to use
	TLSProxyConfig *tls.Config // the [*tls.Config] to use with proxy, if nil, [TLSConfig] will be used
	GetProxy       func(ctx context.Context, r *model.Request) (string, error)
}

func (d *CoreDialer) Clone() Dialer {
	return &CoreDialer{
		TLSConfig: d.TLSConfig,
		GetProxy:  nil,
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
		Key: hp, Dial: func(ctx context.Context) (conn net.Conn, err error) {
			if c, err := tryDialH2(hp); err == nil {
				return c, err
			}
			if d.GetProxy != nil {
				proxy, perr := d.GetProxy(ctx, r.Request)
				if perr != nil {
					return nil, perr
				}
				tlsCfg := d.TLSProxyConfig
				if tlsCfg == nil {
					tlsCfg = d.TLSConfig
				}
				proxyU, perr := url.Parse(proxy)
				if perr != nil {
					return nil, err
				}
				conn, err = dialContextProxy(ctx, r.U, proxyU, tlsCfg)
			} else {
				conn, err = zeroDialer.DialContext(ctx, "tcp", hp)
			}
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
				conn = c
			}
			return conn, nil
		},
	})
}
