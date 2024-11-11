package dialer

import (
	"context"
	"crypto/tls"
	"io"
	"net"

	"github.com/frankli0324/go-http/internal/http"
	"github.com/frankli0324/go-http/internal/transport/h2c"
	"github.com/frankli0324/go-http/utils/netpool"
)

var pool = netpool.NewGroup(100, 80)

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

func (d *CoreDialer) Dial(ctx context.Context, r *http.PreparedRequest) (io.ReadWriteCloser, error) {
	addr, port := r.U.Host, schemes[r.U.Scheme]
	if add, prt, err := net.SplitHostPort(addr); err == nil {
		addr, port = add, prt
	}
	hp := net.JoinHostPort(addr, port)
	re, err := pool.Connect(ctx, netpool.ConnRequest{
		Key: hp, Dial: func(ctx context.Context) (conn net.Conn, err error) {
			conn, err = d.tryDialProxy(ctx, r)
			if err != nil {
				return nil, err
			}
			if conn == nil {
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
				config.ServerName = r.U.Hostname()
				c := tls.Client(conn, config)
				if err := c.HandshakeContext(ctx); err != nil {
					return nil, err
				}
				conn = wrapTLS(c, conn)
				if c.ConnectionState().NegotiatedProtocol == "h2" {
					// must be h2 connection if negotiated h2.
					// if error during handshake, error it is.
					f := h2c.NewConnection(c)
					// [*h2c.Connection] is managed by connection pool this way
					return f, f.Handshake()
				}
			}
			return conn, nil
		},
	})
	if err != nil {
		return nil, err
	}
	if t, ok := re.Raw().(*h2c.Connection); ok {
		// is *tls.Conn or polyfilled TLS Connection
		c, err := t.Stream()
		if err == nil {
			re.Release()
			return (*wStream)(c), err
		} else if ctx.Value(isRetry{}) == nil { // maybe connection fail, try dial new connection
			return d.Dial(context.WithValue(ctx, isRetry{}, true), r)
		}
		return nil, err
	}
	return re, nil
}

type isRetry struct{}

// helper
type wStream h2c.Stream

func (s *wStream) Raw() net.Conn {
	return (*h2c.Stream)(s)
}
