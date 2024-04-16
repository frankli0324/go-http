package internal

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/transport"
)

type ProxyConfig struct {
	TLSConfig      *tls.Config // the [*tls.Config] to use with proxy, if nil, *[CoreDialer.TLSConfig] will be used
	ResolveLocally bool
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

func (d *CoreDialer) tryDialProxy(ctx context.Context, r *model.PreparedRequest) (net.Conn, error) {
	if d.GetProxy != nil {
		proxy, perr := d.GetProxy(ctx, r.Request)
		if perr != nil {
			return nil, perr
		}
		if proxy != "" {
			proxyU, perr := url.Parse(proxy)
			if perr != nil {
				return nil, perr
			}
			return d.DialContextOverProxy(ctx, r.U, proxyU)
		}
	}
	return nil, nil
}

func (d *CoreDialer) DialContextOverProxy(ctx context.Context, remote, proxy *url.URL) (net.Conn, error) {
	if proxy.Scheme != "http" && proxy.Scheme != "https" { // TODO: socks
		return nil, errors.New("unsupported proxy scheme:" + proxy.Scheme)
	}
	hp := proxy.Host
	if proxy.Port() == "" {
		hp = proxy.Hostname() + schemes[proxy.Scheme]
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
		}
		ips, err := d.lookup(ctx, dnsCfg, addr)
		if err != nil {
			return nil, err
		}
		addr = ips[rand.Intn(len(ips))].String()
	}

	connReq := &model.PreparedRequest{
		Request:    &model.Request{Method: "CONNECT"},
		HeaderHost: remote.Host,
		U:          &url.URL{Path: addr + ":" + port},
		GetBody:    func() (io.ReadCloser, error) { return http.NoBody, nil },
	}
	if auth := proxy.User.String(); auth != "" {
		connReq.Header = http.Header{
			"Proxy-Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(auth))},
		}
	}
	if err := h1Transport.Write(ctx, conn, connReq); err != nil {
		conn.Close()
		return nil, err
	}
	resp := &model.Response{}
	if err := h1Transport.Read(ctx, conn, connReq, resp); err != nil {
		conn.Close()
		return nil, err
	}
	if resp.StatusCode != 200 {
		s, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("proxy server returned error. status:%d, body:%s", resp.StatusCode, string(s))
	}
	return conn, nil
}
