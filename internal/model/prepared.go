package model

import (
	"bytes"
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
		Request: r,

		U:             u,
		Header:        headers,
		HeaderHost:    host,
		ContentLength: cl,
	}
	if err := pr.updateBody(); err != nil {
		// note that updateBody potentially updates content-length
		return nil, err
	}
	return pr, nil
}

// should only be called once at [Prepare]
func (r *PreparedRequest) updateBody() (err error) {
	if r.Request.Body == nil {
		r.GetBody = func() (io.ReadCloser, error) {
			return nil, nil
		}
		return nil
	}
	switch b := r.Request.Body.(type) {
	case io.ReadCloser:
		once := atomic.Bool{}
		r.GetBody = func() (io.ReadCloser, error) {
			if once.CompareAndSwap(false, true) {
				return b, nil
			}
			return nil, http.ErrBodyReadAfterClose
		}
		// unknown content-length
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
	default:
		return fmt.Errorf("unsupported body type: %T", r.Request.Body)
	}
	return nil
}
