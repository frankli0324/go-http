package internal

import (
	"context"

	"github.com/frankli0324/go-http/internal/dialer"
	"github.com/frankli0324/go-http/internal/http"
)

type Client struct {
	dialer dialer.Dialer
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
func (c *Client) UseDialer(wrap func(dialer.Dialer) dialer.Dialer) {
	if c.dialer != nil {
		c.dialer = wrap(c.dialer)
	} else {
		c.dialer = wrap(defaultDialer.Clone())
	}
}

func (c *Client) CtxDo(ctx context.Context, req *http.Request) (resp *http.Response, err error) {
	ctx = shadowStandardClientTrace(ctx) // get rid of the httptrace provided by standard library

	pr, err := req.Prepare()
	if err != nil {
		return nil, err
	}

	dialer := c.dialer
	if dialer == nil {
		dialer = defaultDialer
	}
	conn, err := dialer.Dial(ctx, pr)
	if err != nil {
		return nil, err
	}

	resp = new(http.Response)
	err = conn.Do(ctx, pr, resp)
	if err != nil {
		if resp.Body != nil {
			resp.Body.Close()
		}
		resp = nil
	}
	return resp, err
}
