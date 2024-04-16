package internal

import (
	"context"
	"crypto/tls"
	"io"
	"net"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/transport/h2c"
)

type CoreDialer struct {
	ResolveConfig *ResolveConfig

	TLSConfig *tls.Config // the config to use

	GetProxy    func(ctx context.Context, r *model.Request) (string, error)
	ProxyConfig *ProxyConfig
}

func (d *CoreDialer) Clone() Dialer {
	return &CoreDialer{
		ResolveConfig: d.ResolveConfig.Clone(),
		TLSConfig:     d.TLSConfig.Clone(),
		GetProxy:      d.GetProxy,
		ProxyConfig:   d.ProxyConfig.Clone(),
	}
}

func (d *CoreDialer) Unwrap() Dialer {
	return nil
}

var defaultDialer = &CoreDialer{
	TLSConfig: &tls.Config{
		NextProtos: []string{"h2"},
	},
	ProxyConfig: &ProxyConfig{
		TLSConfig:      &tls.Config{}, // don't want h2
		ResolveLocally: false,
	},
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
		if tls, ok := str.Connection.Conn.(*tls.Conn); ok {
			return tls
		}
	}
	return nil
}
