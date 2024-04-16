package internal

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/transport"
)

var (
	h1Transport = transport.HTTP1{}
)

// dialContextProxy creates a connection over http/socks proxy
func dialContextProxy(ctx context.Context, remote, proxy *url.URL, tlsCfg *tls.Config) (net.Conn, error) {
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
		c := tls.Client(conn, tlsCfg)
		if err := c.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = c
	}
	addrport := remote.Host
	if remote.Port() == "" {
		addrport = remote.Hostname() + schemes[remote.Scheme]
	}
	connReq := &model.PreparedRequest{
		Request:    &model.Request{Method: "CONNECT"},
		HeaderHost: remote.Host,
		U:          &url.URL{Path: addrport},
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
