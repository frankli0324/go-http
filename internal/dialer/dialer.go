package dialer

import (
	"context"
	"crypto/tls"

	"github.com/frankli0324/go-http/internal/http"
)

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

func (d *CoreDialer) Unwrap() http.Dialer {
	return nil
}
