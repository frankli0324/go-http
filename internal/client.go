package internal

import (
	"context"
	"io"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/transport"
)

type PreparedRequest = model.PreparedRequest

// Dialers are responsible for creating underlying streams that http requests could
// be written to and responses could be read from. for example, opening a raw TCP
// connection for HTTP/1.1 requests.
//
// Unlike [net/http.Transport], A Dialer MUST NOT hold active connection states,
// which means a Dialer must be able to be swapped out from a [Client] without
// pain. Like [net/http.Transport], it SHOULD hold the connection related configs
// like [ProxyConfiguration] or *[net/tls.Config].
type Dialer interface {
	Dial(ctx context.Context, r *PreparedRequest) (io.ReadWriteCloser, error)
	Unwrap() Dialer
}

type Client struct {
	dialer Dialer
}

// UseDialer provides the interface to modify the dialer used for
// setting up the underlying connections that a request is sent to
// and a response is read from. Connection pooling should be
// implemented at this layer.
//
// This package provides a default dialer type [CoreDialer], which
// uses the pooling logic in the package [netpool]. Users are
// encouraged to re-use the package if they needed to implement
// their own [Dialer], however not necessary. Users could
// get the underlying default [CoreDialer] and modify
// the default logic by [Dialer.Unwrap]ping the given dialer.
//
// For example, http2 can be disabled by removing the "h2" from
// tls ALPN. See how it is be done in [Client.DisableH2].
func (c *Client) UseDialer(wrap func(Dialer) Dialer) {
	if c.dialer != nil {
		c.dialer = wrap(c.dialer)
	} else {
		c.dialer = wrap(defaultDialer.Clone())
	}
}

func (c *Client) dial(ctx context.Context, req *PreparedRequest) (io.ReadWriteCloser, error) {
	if c.dialer != nil {
		return c.dialer.Dial(ctx, req)
	}
	return defaultDialer.Dial(ctx, req)
}

func (c *Client) transport(tlsProto string) transport.Transport {
	if tlsProto == "" { // either not TLS or no protocols negotiated
		return &transport.HTTP1{}
	}
	if tlsProto == "h2" {
		return &transport.H2C{}
	}
	panic("not supported tls proto:" + tlsProto)
}

func (c *Client) CtxDo(ctx context.Context, req *model.Request) (*model.Response, error) {
	pr, err := req.Prepare()
	if err != nil {
		return nil, err
	}
	conn, err := c.dial(ctx, pr)
	if err != nil {
		return nil, err
	}
	proto := ""
	if tls := getTLSConn(conn); tls != nil {
		proto = tls.ConnectionState().NegotiatedProtocol
	}

	tr := c.transport(proto)
	if err := tr.Write(ctx, conn, pr); err != nil {
		return nil, err
	} else {
		resp := &model.Response{}
		return resp, tr.Read(ctx, conn, pr, resp)
	}
}
