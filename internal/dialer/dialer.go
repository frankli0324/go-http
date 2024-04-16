package dialer

import (
	"context"
	"crypto/tls"
	"io"
	"net"

	"github.com/frankli0324/go-http/internal/model"
)

type Conn = net.Conn

// Dialers are responsible for creating underlying streams that http requests could
// be written to and responses could be read from. for example, opening a raw TCP
// connection for HTTP/1.1 requests.
//
// Unlike [net/http.Transport], A Dialer MUST NOT hold active connection states,
// which means a Dialer must be able to be swapped out from a [Client] without
// pain. Like [net/http.Transport], it SHOULD hold the connection related configs
// like [ProxyConfiguration] or *[net/tls.Config].
type Dialer interface {
	Dial(ctx context.Context, r *model.PreparedRequest) (io.ReadWriteCloser, error)
	Unwrap() Dialer
}

type CoreDialer struct {
	ResolveConfig *ResolveConfig

	TLSConfig *tls.Config // the config to use

	GetProxy    func(ctx context.Context, r *model.Request) (string, error)
	ProxyConfig *ProxyConfig
}

func (d *CoreDialer) Clone() *CoreDialer {
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
