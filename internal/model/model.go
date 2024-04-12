package model

import (
	"context"
	"io"
	"net/http"
)

type Handler = func(ctx context.Context) error
type Middleware func(next Handler) Handler

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
