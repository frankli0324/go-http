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
	errs "github.com/frankli0324/go-http/internal/transport/h2c/errors"

	"golang.org/x/net/http2"
)

type H2C struct{}

func (t H2C) RoundTrip(ctx context.Context, rw io.ReadWriteCloser, req *http.PreparedRequest, resp *http.Response) error {
	s, ok := getRawConn(rw).(*h2c.Stream)
	if !ok {
		return errors.New("can only round trip to h2 stream")
	}
	if err := t.WriteRequest(ctx, s, req); err != nil {
		return err
	}
	return t.ReadResponse(ctx, s, req, resp)
}

func (h H2C) ReadResponse(ctx context.Context, s *h2c.Stream, req *http.PreparedRequest, resp *http.Response) error {
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

	resp.Body = s.ResponseBodyStream(ctx)
	return nil
}

func (h H2C) WriteRequest(ctx context.Context, s *h2c.Stream, req *http.PreparedRequest) error {
	stream, err := req.GetBody()
	if err != nil {
		return err
	}
	defer stream.Close()
	hasBody := stream != http.NoBody

	streamID, writtenHeaders := s.Connection.AssignStreamID(s)
	done := make(chan struct{})
	go func() {
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
		writtenHeaders() // can start write next request header
		if err != nil {
			return
		}
		if hasBody {
			err = s.WriteRequestBody(ctx, stream, req.ContentLength, true)
		}
		// TODO: trailers support
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		err = errs.ErrStreamCancelled.Stream(streamID)
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, errs.ErrStreamCancelled) {
		s.Reset(http2.ErrCodeCancel, false)
	} else if errors.Is(err, errs.ErrFramerWrite) {
		s.Reset(http2.ErrCodeProtocol, false)
	} else if err != nil {
		s.Reset(http2.ErrCodeInternal, false)
	}
	return err
}

func getRawConn(c interface{}) net.Conn {
	if conn, ok := c.(interface{ Raw() net.Conn }); ok {
		return conn.Raw()
	}
	return nil
}
