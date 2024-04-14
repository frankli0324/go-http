package transport

import (
	"errors"
	"io"
	"net"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/transport/h2c"
)

type H2C struct{}

func (h *H2C) Read(r io.Reader, req *model.PreparedRequest, resp *model.Response) error {
	_, ok := getRawConn(r).(*h2c.Stream)
	if !ok {
		return errors.New("can only read response from h2 stream")
	}
	panic("unimplemented")
}

func (h *H2C) Write(w io.Writer, req *model.PreparedRequest) error {
	s, ok := getRawConn(w).(*h2c.Stream)
	if !ok {
		return errors.New("can only write request to h2 stream")
	}
	s.Ping()
	// s.Write(req.)
	panic("unimplemented")
}

func getRawConn(c interface{}) net.Conn {
	if conn, ok := c.(interface{ Raw() net.Conn }); ok {
		return conn.Raw()
	}
	return nil
}
