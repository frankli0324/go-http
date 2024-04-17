package internal_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/frankli0324/go-http/internal"
	"github.com/frankli0324/go-http/internal/http"
)

type CombinedReadWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}

type TestDialer struct {
	io.ReadWriteCloser
}

// Dial implements internal.Dialer.
func (t *TestDialer) Dial(ctx context.Context, r *http.PreparedRequest) (io.ReadWriteCloser, error) {
	return t.ReadWriteCloser, nil
}

// Unwrap implements internal.Dialer.
func (t *TestDialer) Unwrap() http.Dialer {
	return nil
}

func SendSingleRequest(t *testing.T, req *http.Request) io.Reader {
	readResponse, writeResponse := io.Pipe()
	go io.Copy(writeResponse, strings.NewReader("HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))

	readRequest, writeRequest := io.Pipe()
	c := &internal.Client{}
	c.UseDialer(func(http.Dialer) http.Dialer {
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
