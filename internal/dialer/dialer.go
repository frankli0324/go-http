package dialer

import (
	"context"
	"crypto/tls"
	"io"

	"github.com/frankli0324/go-http/internal/http"
)

// Dialers handle pretty much everything related to the actual connection,
// including setting a proxy for each request, setting resolvers, etc.
type Dialer interface {
	// Dial returns an abstract stream for writing the request and reading responses.
	// the implementation of this stream could be specific to protocols.
	Dial(ctx context.Context, r *http.PreparedRequest) (io.ReadWriteCloser, error)
	Unwrap() Dialer
}

type CoreDialer struct {
	ResolveConfig *ResolveConfig

	TLSConfig *tls.Config // the config to use

	GetProxy    func(ctx context.Context, r *http.Request) (string, error)
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
