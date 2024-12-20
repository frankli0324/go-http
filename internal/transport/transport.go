package transport

import (
	"context"
	"io"

	"github.com/frankli0324/go-http/internal/http"
)

// Transports holds an actions that Write [http.PreparedRequest]s to an underlying
// [io.WriteCloser] and then Read [http.Response]s from an [io.ReadCloser] which is
// supposedly the same connection the request is sent to. The [Transport]
// implementation should Close the [io.WriteCloser] after the entire request is sent and
// Close the [io.ReadCloser] after the entire response is read, including the request body.
// the Closer for the Response is usually hooked with the Body Closer returned to the user.
//
// Transports SHOULD NOT hold states.
type Transport interface {
	RoundTrip(ctx context.Context, rw io.ReadWriteCloser, req *http.PreparedRequest, resp *http.Response) error
}
