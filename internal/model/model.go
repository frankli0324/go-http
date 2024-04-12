package model

import (
	"io"
	"net/http"
)

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
