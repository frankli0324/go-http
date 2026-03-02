package http1_test

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/frankli0324/go-http/internal"
	"github.com/frankli0324/go-http/internal/dialer"
	"github.com/frankli0324/go-http/internal/http"
	"github.com/frankli0324/go-http/internal/transport/http1"
)

type tCase struct {
	data []byte
	req  *http.Request
}

var reqShouldBe = map[string]tCase{
	"BasicRequest": {
		req: &http.Request{
			Method: "GET",
			URL:    "http://www.example.com",
		},
		data: []byte("GET / HTTP/1.1\r\nHost: www.example.com\r\n\r\n"),
	},
	"RequestWithPort": {
		req: &http.Request{
			Method: "GET",
			URL:    "http://www.example.com:1123",
		},
		data: []byte("GET / HTTP/1.1\r\nHost: www.example.com:1123\r\n\r\n"),
	},
	"QueryNonStandard": {
		req: &http.Request{
			Method: "GET",
			URL:    "http://www.example.com/test?1=33=1",
		},
		data: []byte("GET /test?1=33=1 HTTP/1.1\r\nHost: www.example.com\r\n\r\n"),
	},
	"HeaderNotCanonicalized": {
		req: &http.Request{
			Method: "GET",
			URL:    "http://www.example.com/",
			Header: http.Header{"x-123-vv": {"1"}},
		},
		data: []byte("GET / HTTP/1.1\r\nHost: www.example.com\r\nx-123-vv: 1\r\n\r\n"),
	},
	"URIFragmentNotIncluded": {
		req: &http.Request{
			Method: "GET",
			URL:    "http://www.example.com/?test=1#frag",
		},
		data: []byte("GET /?test=1 HTTP/1.1\r\nHost: www.example.com\r\n\r\n"),
	},
}

func TestRequestSerialize(t *testing.T) {
	for name, cas := range reqShouldBe {
		tCase := cas
		t.Run(name, func(t *testing.T) {
			req := SendSingleRequest(t, tCase.req)
			if err := iotest.TestReader(req, tCase.data); err != nil {
				t.Error(err)
			}
		})
	}
}

type inner struct {
	net.Conn
}
type CombinedReadWriteCloser struct {
	inner
	io.Reader
	io.Writer
	io.Closer
}

type TestDialer struct {
	net.Conn
}

// Dial implements internal.Dialer.
func (t *TestDialer) Dial(ctx context.Context, r *http.PreparedRequest) (http.Conn, error) {
	conn := &http1.Conn{Conn: t.Conn}
	if err := conn.Setup(ctx); err != nil {
		return nil, err
	}
	session, err := conn.Session(ctx, nil)
	if err != nil {
		return nil, err
	}
	return session.(http.Conn), nil
}

// Unwrap implements internal.Dialer.
func (t *TestDialer) Unwrap() dialer.Dialer {
	return nil
}

func SendSingleRequest(t *testing.T, req *http.Request) io.Reader {
	readResponse, writeResponse := io.Pipe()
	go io.Copy(writeResponse, strings.NewReader("HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))

	readRequest, writeRequest := io.Pipe()
	c := &internal.Client{}
	c.UseDialer(func(dialer.Dialer) dialer.Dialer {
		return &TestDialer{CombinedReadWriteCloser{
			Reader: readResponse,
			Writer: writeRequest,
			Closer: writeRequest,
		}}
	})
	go func() {
		resp, err := c.CtxDo(context.Background(), req)
		if err != nil {
			t.Error(err)
		}
		resp.Body.Close()
	}()
	return readRequest
}
