package transport

import (
	"errors"
	"io"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/transport/h2c"
)

type H2C struct{}

func (h *H2C) Read(r io.Reader, req *model.PreparedRequest, resp *model.Response) error {
	_, ok := r.(*h2c.Stream)
	if !ok {
		return errors.ErrUnsupported
	}
	panic("unimplemented")
}

func (h *H2C) Write(w io.Writer, req *model.PreparedRequest) error {
	panic("unimplemented")
}
