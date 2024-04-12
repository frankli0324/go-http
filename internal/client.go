package internal

import (
	"context"
	"io"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/prepare"
	"github.com/frankli0324/go-http/internal/transport"
)

type PreparedRequest = prepare.PreparedRequest

type Client struct {
	middlewares []model.Middleware

	dialer func(ctx context.Context, r *PreparedRequest) (io.ReadWriteCloser, error)
}

// Use appends mw to the end of the chain. The last "Use"d mw executes first
func (c *Client) Use(mws ...model.Middleware) {
	c.middlewares = append(c.middlewares, mws...)
}

func (c *Client) dial(ctx context.Context, req *PreparedRequest) (io.ReadWriteCloser, error) {
	if c.dialer != nil {
		return c.dialer(ctx, req)
	}
	return defaultDial(ctx, req)
}

func (c *Client) CtxDo(ctx context.Context, req *model.Request) (*model.Response, error) {
	pr, err := prepare.Prepare(req)
	if err != nil {
		return nil, err
	}
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
