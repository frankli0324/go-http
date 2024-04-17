package http

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
)

type PreparedRequest struct {
	*Request

	U          *url.URL
	GetBody    func() (io.ReadCloser, error)
	Header     http.Header
	HeaderHost string

	ContentLength int64
}

func (r *Request) Prepare() (*PreparedRequest, error) {
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, err
	}

	headers := r.Header.Clone()
	host := u.Host
	cl := int64(-1)
	// user defined headers has higher priority
	for k, v := range headers {
		if strings.ToLower(k) == "host" {
			if len(v) != 0 { // && !httpguts.ValidHostHeader(host)
				host = v[0]
			}
			delete(headers, k)
		}

		if strings.ToLower(k) == "content-length" {
			if len(v) != 0 {
				if v, err := strconv.ParseInt(v[0], 10, 64); err == nil {
					cl = v
				}
			}
			delete(headers, k)
		}
	}
	if host == "" {
		return nil, url.InvalidHostError("empty host")
	}

	pr := &PreparedRequest{
		Request: r, U: u,
		Header: headers, HeaderHost: host,
		ContentLength: cl,
	}
	if err := pr.updateBody(); err != nil {
		// note that updateBody potentially updates content-length
		return nil, err
	}
	if cl != -1 && pr.ContentLength != cl {
		return nil, errors.New("conflicting value between body size and content-length request header")
	}
	return pr, nil
}

// should only be called once at [Prepare]
func (r *PreparedRequest) updateBody() (err error) {
	if r.Request.Body == nil {
		r.GetBody = func() (io.ReadCloser, error) {
			return http.NoBody, nil
		}
		return nil
	}
	switch b := r.Request.Body.(type) {
	case string:
		r.ContentLength = int64(len(b))
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(b)), nil
		}
	case []byte:
		r.ContentLength = int64(len(b))
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(b)), nil
		}
	case *bytes.Buffer: // below is taken from http.NewRequest
		r.ContentLength = int64(b.Len())
		buf := b.Bytes()
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(buf)), nil
		}
	case *bytes.Reader:
		r.ContentLength = int64(b.Len())
		snapshot := *b
		r.GetBody = func() (io.ReadCloser, error) {
			r := snapshot
			return io.NopCloser(&r), nil
		}
	case *strings.Reader:
		r.ContentLength = int64(b.Len())
		snapshot := *b
		r.GetBody = func() (io.ReadCloser, error) {
			r := snapshot
			return io.NopCloser(&r), nil
		}
	case io.Reader:
		if sizer, ok := b.(interface{ Size() int64 }); ok {
			r.ContentLength = sizer.Size()
		}
		cb, ok := b.(io.ReadCloser)
		if !ok {
			cb = io.NopCloser(b)
		}
		once := uint32(0)
		r.GetBody = func() (io.ReadCloser, error) {
			if atomic.CompareAndSwapUint32(&once, 0, 1) {
				return cb, nil
			}
			return nil, http.ErrBodyReadAfterClose
		}
	default:
		return fmt.Errorf("unsupported body type: %T", r.Request.Body)
	}
	return nil
}
