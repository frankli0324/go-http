package http

import (
	"context"
	"io"
	"net/http"
)

// Request is an object holding minimal information a request contains.
// it should not contain connection related information, such as
// proxies, context, response, connection info and "Close". it should
// be handled instead by [Dialer]s.
type Request struct {
	Method string
	URL    string
	Body   interface{}
	Header http.Header
}

type Response struct {
	Proto      string
	Status     string
	StatusCode int
	Header     http.Header

	ContentLength    int64
	TransferEncoding string

	Body io.ReadCloser
}

type Conn interface {
	Do(context.Context, *PreparedRequest, *Response) error
}
