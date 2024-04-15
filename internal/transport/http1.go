package transport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/frankli0324/go-http/internal/model"
	"github.com/frankli0324/go-http/internal/transport/chunked"
)

type HTTP1 struct{}

func (t *HTTP1) Write(w io.Writer, r *model.PreparedRequest) error {
	body, err := r.GetBody() // can write body
	if err != nil {
		return err
	}
	if body != nil {
		defer body.Close() // request body is ALWAYS closed
	}

	if body != http.NoBody && r.ContentLength == -1 {
		r.Header.Set("Transfer-Encoding", "chunked")
	}
	if err := t.writeHeader(w, r); err != nil {
		return err
	}
	if body == http.NoBody {
		return nil
	}
	if r.ContentLength == -1 {
		cw := chunked.NewChunkedWriter(w)
		if _, err := io.Copy(cw, body); err != nil {
			return err
		}
		if err := cw.CloseWithTrailer(nil); err != nil {
			return err
		}
	} else {
		n, err := io.Copy(w, body)
		if err != nil {
			return err
		}
		if n != r.ContentLength {
			return io.ErrShortWrite
		}
	}

	return nil
}

// mimic stdlib behavior
func (t *HTTP1) expectContentLength(r *model.PreparedRequest) bool {
	if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
		return true
	}
	if r.Method == "CONNECT" {
		return false
	}
	if r.Header.Get("Transfer-Encoding") == "identity" {
		if r.Method == "GET" || r.Method == "HEAD" {
			return false
		}
		return true
	}
	return false
}

// writeHeader writes the status and header part of an http 1.1 request
// e.g.:
//
//	GET / HTTP/1.1\r\n
//	Host: www.google.com\r\n
//	X-Xx-Yy: cccccc\r\n
//	\r\n
func (t *HTTP1) writeHeader(w io.Writer, r *model.PreparedRequest) error {
	header := bufio.NewWriter(w) // default bufsize is 4096

	if _, err := header.WriteString(r.Method); err != nil {
		return err
	}
	header.WriteByte(' ')
	header.WriteString(r.U.RequestURI())
	header.WriteString(" HTTP/1.1\r\n")
	if err := header.Flush(); err != nil {
		return err
	}

	header.WriteString("Host: ")
	header.WriteString(r.HeaderHost)
	header.WriteString("\r\n")
	if r.ContentLength > 0 {
		header.WriteString("Content-Length: ")
		header.WriteString(strconv.FormatInt(r.ContentLength, 10))
		header.WriteString("\r\n")
	} else if r.ContentLength == 0 && t.expectContentLength(r) {
		header.WriteString("Content-Length: 0\r\n")
	}

	for k, v := range r.Header {
		for _, v := range v {
			header.WriteString(k)
			header.WriteString(": ")
			header.WriteString(v)
			if _, err := header.WriteString("\r\n"); err != nil {
				return err
			}
		}
	}
	if _, err := header.WriteString("\r\n"); err != nil {
		return err
	}
	if err := header.Flush(); err != nil {
		return err
	}
	return nil
}

func (t *HTTP1) Read(r io.Reader, req *model.PreparedRequest, resp *model.Response) (err error) {
	tp := textproto.NewReader(bufio.NewReader(r))
	if err := t.readHeader(tp, resp); err != nil {
		return err
	}

	// A client MUST ignore any Content-Length or Transfer-Encoding header fields
	// received in a successful response to CONNECT.
	// https://www.rfc-editor.org/rfc/rfc9110#section-9.3.6-12
	if req.Method == "CONNECT" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		resp.Header.Del("Transfer-Encoding")
		resp.Header.Del("Content-Length")
	}

	return t.readTransfer(tp.R, r, req, resp)
}

func (t *HTTP1) readHeader(tp *textproto.Reader, resp *model.Response) error {
	line, err := tp.ReadLine()
	if err != nil {
		if err == io.EOF {
			err = fmt.Errorf("unexpected error while reading response headers: %w", io.ErrUnexpectedEOF)
		}
		return err
	}
	proto, status, ok := Cut(line, " ")
	if !ok {
		return errors.New("malformed HTTP response")
	}
	resp.Proto = proto
	resp.Status = strings.TrimLeft(status, " ")

	statusCode, _, _ := Cut(resp.Status, " ")
	if len(statusCode) != 3 {
		return errors.New("malformed HTTP status code " + statusCode)
	}
	resp.StatusCode, err = strconv.Atoi(statusCode)
	if err != nil || resp.StatusCode < 0 {
		return errors.New("malformed HTTP status code")
	}

	// Parse the response headers. There are cases where case sensitivity
	// is needed, but is ignored intentionally in this implementation.
	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	if hp, ok := mimeHeader["Pragma"]; ok && len(hp) > 0 && hp[0] == "no-cache" {
		if _, presentcc := mimeHeader["Cache-Control"]; !presentcc {
			mimeHeader["Cache-Control"] = []string{"no-cache"}
		}
	}
	resp.Header = http.Header(mimeHeader)
	return nil
}

func (t *HTTP1) readTransfer(br, r io.Reader, req *model.PreparedRequest, resp *model.Response) error {
	closer := io.NopCloser
	if cr, ok := r.(Releaser); ok {
		closer = func(r io.Reader) io.ReadCloser {
			return bodyCloser{r, func() error {
				cr.Release()
				return nil
			}}
		}
	}
	if cr, ok := r.(io.Closer); ok {
		if req.Header.Get("Connection") == "close" || resp.Header.Get("Connection") == "close" {
			closer = func(r io.Reader) io.ReadCloser { return bodyCloser{r, cr.Close} }
		}
	}

	// the header key was canonicalized while reading from the stream
	contentLens := resp.Header["Content-Length"]
	delete(resp.Header, "Content-Length")

	// Hardening against HTTP request smuggling, taken from standard library
	if len(contentLens) > 1 {
		// Per RFC 7230 Section 3.3.2
		first := textproto.TrimString(contentLens[0])
		for _, ct := range contentLens[1:] {
			if first != textproto.TrimString(ct) {
				return fmt.Errorf("http: message cannot contain multiple Content-Length headers; got %q", contentLens)
			}
		}
		contentLens[0] = first
	}

	if resp.Header.Get("Transfer-Encoding") == "chunked" {
		resp.Body = closer(chunked.NewChunkedReader(br))
		resp.Header.Del("Transfer-Encoding")
		return nil
	}

	resp.ContentLength = -1
	if len(contentLens) > 0 {
		// Logic based on Content-Length
		n, err := strconv.ParseUint(contentLens[0], 10, 63)
		if err != nil {
			return errors.New("invalid content-length response header: " + contentLens[0])
		}
		resp.ContentLength = int64(n)
	} else { // not chunked encoding, no content-length present, assume connection: close
		if cr, ok := r.(io.Closer); ok {
			closer = func(r io.Reader) io.ReadCloser { return bodyCloser{r, cr.Close} }
		}
	}

	switch {
	case resp.ContentLength > 0:
		resp.Body = closer(io.LimitReader(br, resp.ContentLength))
	case resp.ContentLength == 0:
		closer(nil).Close()
		resp.Body = http.NoBody
	case resp.ContentLength < 0: // no content-length header present, assume connection: close
		resp.Body = closer(br)
	}
	return nil
}
