package internal

import (
	"context"
	"io"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/transport"
)

type PreparedRequest = model.PreparedRequest

type Handler = func(ctx context.Context, req *PreparedRequest) (*model.Response, error)
type Middleware func(next Handler) Handler

type Dialer interface {
	Dial(ctx context.Context, r *PreparedRequest) (io.ReadWriteCloser, error)
	Unwrap() Dialer
}

type Client struct {
	middlewares []Middleware
	dialer      Dialer
}

// Use appends mw to the end of the chain. The last "Use"d mw executes first
func (c *Client) Use(mws ...Middleware) {
	c.middlewares = append(c.middlewares, mws...)
}

func (c *Client) UseDialer(wrap func(Dialer) Dialer) {
	if c.dialer != nil {
		c.dialer = wrap(c.dialer)
	} else {
		c.dialer = wrap(defaultDialer)
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
	next := func(ctx context.Context, req *PreparedRequest) (*model.Response, error) {
		conn, err := c.dial(ctx, pr)
		if err != nil {
			return nil, err
		}
		proto := ""
		if tls := getTLSConn(conn); tls != nil {
			proto = tls.ConnectionState().NegotiatedProtocol
		}
		tr := c.transport(proto)
		if err := tr.Write(conn, pr); err != nil {
			return nil, err
		} else {
			resp := &model.Response{}
			return resp, tr.Read(conn, resp)
		}
	}
	for i := len(c.middlewares) - 1; i > 1; i-- {
		next = c.middlewares[i](next)
	}
	return next(ctx, pr)
}
