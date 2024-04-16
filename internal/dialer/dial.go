package dialer

import (
	"context"
	"crypto/tls"
	"io"
	"net"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/netpool"
)

var pool = netpool.NewGroup(100, 80)

var schemes = map[string]string{
	"http": "80", "https": "443", "socks": "1080",
}

var zeroDialer net.Dialer
var customDnsDialer = net.Dialer{
	Resolver: &customServerResolver,
}

func (d *CoreDialer) Dial(ctx context.Context, r *model.PreparedRequest) (io.ReadWriteCloser, error) {
	addr, port := r.U.Host, schemes[r.U.Scheme]
	if add, prt, err := net.SplitHostPort(addr); err == nil {
		addr, port = add, prt
	}
	hp := net.JoinHostPort(addr, port)
	return pool.Connect(ctx, netpool.ConnRequest{
		Key: hp, Dial: func(ctx context.Context) (conn net.Conn, err error) {
			if c, err := tryDialH2(hp); err == nil {
				return c, err
			}

			conn, err = d.tryDialProxy(ctx, r)
			if err != nil {
				return nil, err
			}
			if conn == nil {
				// if needCustomDial(d.ResolveConfig) {}
				// as of now net.Dialer could handle current DNS configurations
				network, dialer, dialctx, dst := "tcp", &zeroDialer, ctx, hp

				if d.ResolveConfig.Network == "ip4" {
					network = "tcp4"
				} else if d.ResolveConfig.Network == "ip6" {
					network = "tcp6"
				}
				if static, ok := d.ResolveConfig.StaticHosts[addr]; ok {
					dst = net.JoinHostPort(static, port)
				}
				if dns := d.ResolveConfig.CustomDNSServer; dns != "" {
					dialctx = dnsServerCtx{dialctx, dns}
					dialer = &customDnsDialer
				}

				conn, err = dialer.DialContext(dialctx, network, dst)
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
