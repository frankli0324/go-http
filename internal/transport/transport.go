package transport

import (
	"io"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/prepare"
)

type Transport interface {
	Read(r io.Reader, resp *model.Response) error
	Write(w io.Writer, req *prepare.PreparedRequest) error
}

var defaultTransport = &http1{}

func Read(r io.Reader, resp *model.Response) error {
	return defaultTransport.Read(r, resp)
}
func Write(w io.Writer, req *prepare.PreparedRequest) error {
	return defaultTransport.Write(w, req)
}
