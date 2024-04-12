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

type Dialer func(ctx context.Context, r *PreparedRequest) (io.ReadWriteCloser, error)

type Client struct {
	middlewares []Middleware
	dialer      Dialer
}

// Use appends mw to the end of the chain. The last "Use"d mw executes first
func (c *Client) Use(mws ...Middleware) {
	c.middlewares = append(c.middlewares, mws...)
}

func (c *Client) dial(ctx context.Context, req *PreparedRequest) (io.ReadWriteCloser, error) {
	if c.dialer != nil {
		return c.dialer(ctx, req)
	}
	return defaultDial(ctx, req)
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
		if err := transport.Write(conn, pr); err != nil {
			return nil, err
		} else {
			resp := &model.Response{}
			return resp, transport.Read(conn, resp)
		}
	}
	for i := len(c.middlewares) - 1; i > 1; i-- {
		next = c.middlewares[i](next)
	}
	return next(ctx, pr)
}
