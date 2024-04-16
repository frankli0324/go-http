package transport

import (
	"context"
	"io"

	"github.com/frankli0324/go-http/internal/model"
)

type Transport interface {
	Read(ctx context.Context, r io.Reader, req *model.PreparedRequest, resp *model.Response) error
	Write(ctx context.Context, w io.Writer, req *model.PreparedRequest) error
}
