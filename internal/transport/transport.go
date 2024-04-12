package transport

import (
	"io"

	"github.com/frankli0324/go-http/internal/model"
)

type Transport interface {
	Read(r io.Reader, resp *model.Response) error
	Write(w io.Writer, req *model.PreparedRequest) error
}
