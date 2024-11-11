package transport

import (
	"context"
	"errors"
	"io"
	"net"
	nhttp "net/http"
	"strconv"

	"github.com/frankli0324/go-http/internal/http"
	"github.com/frankli0324/go-http/internal/transport/h2c"
)

type H2C struct{}

func (t H2C) RoundTrip(ctx context.Context, rw io.ReadWriteCloser, req *http.PreparedRequest, resp *http.Response) error {
	if err := t.WriteRequest(ctx, rw, req); err != nil {
		return err
	}
	return t.ReadResponse(ctx, rw, req, resp)
}

func (h H2C) ReadResponse(ctx context.Context, r io.ReadCloser, req *http.PreparedRequest, resp *http.Response) error {
	s, ok := getRawConn(r).(*h2c.Stream)
	if !ok {
		return errors.New("can only read response from h2 stream")
	}

	resp.Header = make(http.Header)
	err := s.ReadHeaders(ctx, func(k, v string) error {
		if len(k) > 0 && k[0] == ':' {
			switch k {
			case ":status":
				code, err := strconv.Atoi(v)
				if err != nil {
					return err
				}
				resp.StatusCode = code
				resp.Status = v + " " + nhttp.StatusText(code)
			default:
				return errors.New("invalid response header")
			}
		} else {
			resp.Header.Add(k, v)
		}
		return nil
	})
	if err != nil {
		return err
	}

	resp.Body = bodyCloser{
		s.ResponseBodyStream(ctx),
		func() error { return r.Close() },
	}
	return nil
}

func (h H2C) WriteRequest(ctx context.Context, w io.Writer, req *http.PreparedRequest) error {
	s, ok := getRawConn(w).(*h2c.Stream)
	if !ok {
		return errors.New("can only write request to h2 stream")
	}
	stream, err := req.GetBody()
	if err != nil {
		return err
	}
	defer stream.Close()
	hasBody := stream != http.NoBody

	done := s.Connection.AssignStreamID(s)
	err = s.WriteHeaders(ctx, func(f func(k, v string)) {
		f(":method", req.Method)
		f(":authority", req.HeaderHost)
		if req.Method != "CONNECT" {
			f(":scheme", req.U.Scheme)
			f(":path", req.U.RequestURI())
		}
		for k, v := range req.Header {
			for _, v := range v {
				f(k, v)
			}
		}
		if hasBody && req.ContentLength != -1 {
			f("content-length", strconv.FormatInt(req.ContentLength, 10))
		}
	}, !hasBody /* && request has no trailers */)
	done() // can start write next request header
	if err != nil {
		return err
	}
	if hasBody {
		err = s.WriteRequestBody(ctx, stream, req.ContentLength, true)
	}
	// TODO: trailers support
	return err
}

func getRawConn(c interface{}) net.Conn {
	if conn, ok := c.(interface{ Raw() net.Conn }); ok {
		return conn.Raw()
	}
	return nil
}
