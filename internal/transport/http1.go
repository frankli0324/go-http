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

	if err := t.writeHeader(w, r); err != nil {
		return err
	}
	if body != nil {
		if _, err := io.Copy(w, body); err != nil {
			return err
		}
	}

	return nil
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
	if r.ContentLength != -1 {
		header.WriteString("Content-Length: ")
		header.WriteString(strconv.FormatInt(r.ContentLength, 10))
		header.WriteString("\r\n")
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
	closer := io.NopCloser
	if cr, ok := r.(Releaser); ok {
		closer = func(r io.Reader) io.ReadCloser {
			return bodyCloser{r, func() error {
				cr.Release()
				return nil
			}}
		}
	}
	if cr, ok := r.(io.Closer); ok && req.Header.Get("Connection") == "close" {
		closer = func(r io.Reader) io.ReadCloser { return bodyCloser{r, cr.Close} }
	}
	tp := textproto.NewReader(bufio.NewReader(r))

	line, err := tp.ReadLine()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	proto, status, ok := strings.Cut(line, " ")
	if !ok {
		return errors.New("malformed HTTP response")
	}
	resp.Proto = proto
	resp.Status = strings.TrimLeft(status, " ")

	statusCode, _, _ := strings.Cut(resp.Status, " ")
	if len(statusCode) != 3 {
		return errors.New("malformed HTTP status code " + statusCode)
	}
	resp.StatusCode, err = strconv.Atoi(statusCode)
	if err != nil || resp.StatusCode < 0 {
		return errors.New("malformed HTTP status code")
	}

	// Parse the response headers.
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

	if cr, ok := r.(io.Closer); ok && resp.Header.Get("Connection") == "close" {
		closer = func(r io.Reader) io.ReadCloser { return bodyCloser{r, cr.Close} }
	}

	return t.readTransfer(tp.R, resp, closer)
}

func (t *HTTP1) readTransfer(r io.Reader, resp *model.Response, closer func(io.Reader) io.ReadCloser) error {
	contentLens := resp.Header["Content-Length"]

	// Hardening against HTTP request smuggling, taken from standard library
	if len(contentLens) > 1 {
		// Per RFC 7230 Section 3.3.2
		first := textproto.TrimString(contentLens[0])
		for _, ct := range contentLens[1:] {
			if first != textproto.TrimString(ct) {
				return fmt.Errorf("http: message cannot contain multiple Content-Length headers; got %q", contentLens)
			}
		}

		// deduplicate Content-Length
		resp.Header.Del("Content-Length")
		resp.Header.Add("Content-Length", first)

		contentLens = resp.Header["Content-Length"]
	}

	cl := int64(-1)
	if len(contentLens) > 0 {
		// Logic based on Content-Length
		n, err := strconv.ParseUint(contentLens[0], 10, 63)
		if err == nil {
			cl = int64(n)
		}
	}

	if resp.Header.Get("Transfer-Encoding") == "chunked" {
		resp.Body = closer(chunked.NewChunkedReader(r))
		return nil
	}

	resp.Header.Del("Content-Length")
	resp.ContentLength = cl
	switch {
	case cl > 0:
		resp.Body = closer(io.LimitReader(r, cl))
	case cl == 0:
		closer(nil).Close()
		resp.Body = http.NoBody
	}
	return nil
}
