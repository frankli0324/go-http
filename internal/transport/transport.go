package transport

import (
	"context"
	"io"

	"github.com/frankli0324/go-http/internal/http"
)

type Transport interface {
	Read(ctx context.Context, r io.ReadCloser, req *http.PreparedRequest, resp *http.Response) error
	Write(ctx context.Context, w io.WriteCloser, req *http.PreparedRequest) error
}
