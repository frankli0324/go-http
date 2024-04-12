package transport

import (
	"io"

	"github.com/frankli0324/go-http/internal/model"
)

type H2C struct{}

func (h *H2C) Read(r io.Reader, resp *model.Response) error {
	panic("unimplemented")
}

func (h *H2C) Write(w io.Writer, req *model.PreparedRequest) error {
	panic("unimplemented")
}
