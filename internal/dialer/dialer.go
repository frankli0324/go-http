package dialer

import (
	"context"
	"crypto/tls"
	"io"

	"github.com/frankli0324/go-http/internal/model"
)

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
