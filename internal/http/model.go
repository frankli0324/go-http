package http

import (
	"context"
	"io"
	"net/http"
)

type Dialer interface {
	Dial(ctx context.Context, r *PreparedRequest) (io.ReadWriteCloser, error)
	Unwrap() Dialer
}

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

	ContentLength int64
	Body          io.ReadCloser
}
