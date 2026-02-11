package dialer

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"

	"github.com/frankli0324/go-http/internal/http"
	"github.com/frankli0324/go-http/internal/transport"
)

type ProxyConfig struct {
	TLSConfig      *tls.Config    // the [*tls.Config] to use with proxy, if nil, *[CoreDialer.TLSConfig] will be used
	ResolveLocally bool           // resolve the hostname though DNS before dialing through proxy
	ResolveConfig  *ResolveConfig // overrides the resolver config for dialer for proxy
}

func (c *ProxyConfig) Clone() *ProxyConfig {
	if c == nil {
		return nil
	}
	return &ProxyConfig{
		TLSConfig:      c.TLSConfig.Clone(),
		ResolveLocally: c.ResolveLocally,
		ResolveConfig:  c.ResolveConfig.Clone(),
	}
}

var (
	h1Transport = transport.HTTP1{}
)

// DialContextOverProxy creates a connection over http/socks proxy.
// This part of logic may be reused when wrapping *[CoreDialer] into
// a new custom [Dialer]
func (d *CoreDialer) DialContextOverProxy(ctx context.Context, remote, proxy *url.URL) (net.Conn, error) {
	if proxy.Scheme != "http" && proxy.Scheme != "https" { // TODO: socks
		return nil, errors.New("unsupported proxy scheme:" + proxy.Scheme)
	}
	hp := proxy.Host
	if proxy.Port() == "" {
		hp = proxy.Hostname() + ":" + schemes[proxy.Scheme]
	}

	conn, err := zeroDialer.DialContext(ctx, "tcp", hp)
	if err != nil {
		return nil, err
	}

	if proxy.Scheme == "https" {
		tlsCfg := d.ProxyConfig.TLSConfig
		if tlsCfg == nil {
			tlsCfg = d.TLSConfig
		}
		c := tls.Client(conn, tlsCfg)
		if err := c.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = c
	}

	addr, port := remote.Host, schemes[remote.Scheme]
	if add, prt, err := net.SplitHostPort(addr); err == nil {
		addr, port = add, prt
	}

	if d.ProxyConfig.ResolveLocally {
		dnsCfg := d.ProxyConfig.ResolveConfig
		if dnsCfg == nil {
			dnsCfg = d.ResolveConfig
		} else {
			dnsCfg = dnsCfg.Merge(d.ResolveConfig)
		}

		if res, ok := dnsCfg.StaticHosts[addr]; ok {
			addr = res
		} else {
			ips, err := d.lookup(ctx, dnsCfg, addr)
			if err != nil {
				return nil, err
			}
			addr = ips[rand.Intn(len(ips))].String()
		}
	}

	// TODO: implement CONNECT over http2
	// however in most cases, it seems that using a stream instead of an entire
	// tcp connection for proxied connections is less efficient. h2 over h2 proxy
	// maintains unnecessary state between user agent and proxy, while h1 over h2
	// proxy is less common (which is the only sane scenario).
	//
	// also it's only when we also maintain agent->proxy connections in connection
	// pools that dialing over h2 proxies is meaningful.
	connReq := &http.PreparedRequest{
		Request:    &http.Request{Method: "CONNECT"},
		HeaderHost: proxy.Host,
		U:          &url.URL{Path: addr + ":" + port},
		GetBody:    func() (io.ReadCloser, error) { return http.NoBody, nil },
	}
	if auth := proxy.User.String(); auth != "" {
		connReq.Header = http.Header{
			"Proxy-Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(auth))},
		}
	}
	resp := &http.Response{}
	if err := h1Transport.RoundTrip(ctx, conn, connReq, resp); err != nil {
		conn.Close()
		return nil, err
	}
	if status := resp.StatusCode; status != 200 {
		s, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("proxy server returned error. status:%d, body:%s", status, string(s))
	}
	return conn, nil
}
