package dialer

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"

	"github.com/frankli0324/go-http/internal/http"
	"github.com/frankli0324/go-http/internal/transport/http1"
	"github.com/frankli0324/go-http/utils/netpool"
)

var schemes = map[string]string{
	"http": "80", "https": "443", "socks": "1080",
}

var zeroDialer net.Dialer
var customDnsDialer = net.Dialer{
	Resolver: &customServerResolver,
}

func (d *CoreDialer) dialRaw(ctx context.Context, addr, port string) (net.Conn, error) {
	// if needCustomDial(d.ResolveConfig) {}
	// as of now net.Dialer could handle current DNS configurations
	if d.ResolveConfig == nil {
		return zeroDialer.DialContext(ctx, "tcp", net.JoinHostPort(addr, port))
	}
	network, dialer, dialctx, dst := "tcp", &zeroDialer, ctx, ""

	if d.ResolveConfig.Network == "ip4" {
		network = "tcp4"
	} else if d.ResolveConfig.Network == "ip6" {
		network = "tcp6"
	}
	if static, ok := d.ResolveConfig.StaticHosts[addr]; ok {
		dst = net.JoinHostPort(static, port)
	} else {
		dst = net.JoinHostPort(addr, port)
	}
	if dns := d.ResolveConfig.CustomDNSServer; dns != "" {
		dialctx = dnsServerCtx{dialctx, dns}
		dialer = &customDnsDialer
	}

	return dialer.DialContext(dialctx, network, dst)
}

func (d *CoreDialer) Dial(ctx context.Context, r *http.PreparedRequest) (http.Conn, error) {
	addr, port := r.U.Host, schemes[r.U.Scheme]
	if add, prt, err := net.SplitHostPort(addr); err == nil {
		addr, port = add, prt
	}

	var proxy string
	if d.GetProxy != nil {
		var err error
		proxy, err = d.GetProxy(ctx, r.Request)
		if err != nil {
			return nil, err
		}
	}
	re, err := d.ConnPool.Connect(ctx, dialKey{addr, port, proxy},
		func(ctx context.Context) (netpool.Conn, error) {
			var conn net.Conn
			var err error
			if proxy != "" {
				purl, perr := url.Parse(proxy)
				if perr != nil {
					return nil, perr
				}
				conn, err = d.DialContextOverProxy(ctx, r.U, purl)
			} else {
				conn, err = d.dialRaw(ctx, addr, port)
			}
			if err != nil {
				return nil, err
			}
			if r.U.Scheme == "https" {
				config := d.TLSConfig.Clone()
				if config == nil {
					config = &tls.Config{}
				}
				if config.ServerName == "" {
					config.ServerName = r.U.Hostname()
				}
				c := tls.Client(conn, config)
				if err := c.HandshakeContext(ctx); err != nil {
					return nil, err
				}
				conn = wrapTLS(c, conn)
				// TODO: bring back http2
			}
			return &http1.Conn{Conn: conn}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return re.(http.Conn), nil
}

type dialKey struct {
	host, port, proxy string
}
