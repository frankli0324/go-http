package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/transport/h2c"
)

type H2C struct{}

func (h *H2C) Read(ctx context.Context, r io.Reader, req *model.PreparedRequest, resp *model.Response) error {
	s, ok := getRawConn(r).(*h2c.Stream)
	if !ok {
		return errors.New("can only read response from h2 stream")
	}
	fmt.Println(s.ID())
	panic("unimplemented")
}

func (h *H2C) Write(ctx context.Context, w io.Writer, req *model.PreparedRequest) error {
	s, ok := getRawConn(w).(*h2c.Stream)
	if !ok {
		return errors.New("can only write request to h2 stream")
	}
	stream, err := req.GetBody()
	if err != nil {
		return err
	}
	defer stream.Close()

	if err := s.WriteHeaders(ctx, func(f func(k, v string)) {
		f(":method", req.Method)
		f(":authority", req.HeaderHost)
		if req.Method != "CONNECT" {
			f(":scheme", req.U.Scheme)
			f(":path", req.U.RequestURI())
		}
	}, stream == http.NoBody /* && request has no trailers */); err != nil {
		// TODO: trailers support
		return err
	}
	if stream != http.NoBody {
		// s.Write()
	}

	return nil
}

func getRawConn(c interface{}) net.Conn {
	if conn, ok := c.(interface{ Raw() net.Conn }); ok {
		return conn.Raw()
	}
	return nil
}
