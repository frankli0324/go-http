package http1

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"sync"

	"github.com/frankli0324/go-http/internal/http"
	"github.com/frankli0324/go-http/internal/transport/chunked"
	"github.com/frankli0324/go-http/utils/netpool"
)

type Session struct {
	Sess netpool.Session // could be nil if connection is not a session, e.g. proxy dial
	c    *Conn

	ctx  context.Context
	req  *http.PreparedRequest
	resp *http.Response

	needClose bool
	reader    io.Reader
	readraw   bool
	remaining int64 // initially set to content-length

	doneCh     chan error    // doneCh
	bodyClosed chan struct{} // Signal when body is closed
}

func (s *Session) Do(ctx context.Context, req *http.PreparedRequest, resp *http.Response) error {
	s.ctx, s.req, s.resp = ctx, req, resp
	s.doneCh = make(chan error, 1)
	s.bodyClosed = make(chan struct{}, 1)

	select {
	case s.c.wloop <- s:
		return <-s.doneCh
	case <-ctx.Done():
		return ctx.Err()
	}
}

// implements netpool.Session
func (s *Session) Release(close bool) (reused bool, err error) {
	if s.Sess == nil {
		return
	}
	reused, err = s.Sess.Release(close)
	s.Sess = nil
	return
}

func (s *Session) Read(buf []byte) (read int, err error) {
	if s.remaining == 0 {
		return 0, io.EOF
	}
	if s.remaining > 0 && s.remaining < int64(len(buf)) {
		buf = buf[:s.remaining]
	}
	if s.readraw {
		read, err = s.ReadRaw(buf)
	} else {
		read, err = s.reader.Read(buf)
	}
	if s.remaining > 0 {
		s.remaining -= int64(read)
	}
	return
}
func (s *Session) ReadRaw(buf []byte) (read int, err error) {
	rem := s.c.Reader
	if n := rem.Buffered(); n != 0 {
		if n >= len(buf) {
			return rem.Read(buf)
		}
		read, err = rem.Read(buf[:n])
		if read != n || err != nil {
			panic("bufio.Read into buffer larger than b.Buffered() failed")
			// should not be here
		}
		if len(buf)-n < 32 { // existing bufio.Reader almost fills buf up
			return
		}
		buf = buf[n:]
	}
	n, err := s.c.Conn.Read(buf)
	return read + n, err
}

// implements ReadCloser
func (s *Session) Close() (err error) {
	if s.Sess != nil {
		s.Sess.Release(s.needClose)
		s.Sess = nil
	}
	if s.needClose {
		err = s.c.Close()
	}
	return
}
func (s *Session) writeRequest(_ context.Context) error {
	r, c := s.req, s.c.Conn
	body, err := r.GetBody() // can write body
	if err != nil {
		return err
	}
	if body == nil {
		return errors.New("invalid request: body is nil after prepare")
	}
	defer body.Close() // request body is ALWAYS closed

	if body != http.NoBody && r.ContentLength == -1 {
		r.Header.Set("Transfer-Encoding", "chunked")
	}
	if err := writeHeader(c, r); err != nil {
		return err
	}
	if body == http.NoBody {
		return nil
	}
	if r.ContentLength == -1 {
		cw := chunked.NewChunkedWriter(c)
		if _, err := io.Copy(cw, body); err != nil {
			return err
		}
		if err := cw.CloseWithTrailer(nil); err != nil {
			return err
		}
	} else {
		n, err := io.Copy(c, body)
		if err != nil {
			return err
		}
		if n != r.ContentLength {
			return io.ErrShortWrite
		}
	}
	return nil
}
func (s *Session) readResponse(ctx context.Context) (err error) {
	s.needClose, err = readHeader(ctx, s.c.Reader, s.req, s.resp)
	s.remaining = s.resp.ContentLength
	switch {
	case s.resp.TransferEncoding != "":
		s.reader = s.c.Reader
		// TODO: maybe reading directly from net.Conn is more efficient if going to support more encodings
		walkReverse(s.resp.TransferEncoding, func(enc string) bool {
			switch enc {
			// apply decoder
			case "chunked":
				s.reader = chunked.NewChunkedReader(s.reader)
			default:
				err = errors.New("unsupported transfer-encoding")
			}
			return true
		})
	case s.resp.ContentLength > 0:
		s.readraw = true
	}
	s.resp.Body = s
	return
}

type Conn struct {
	net.Conn
	Reader *bufio.Reader

	wloop, rloop chan *Session
	werr, rerr   error
}

func (c *Conn) Session(ctx context.Context, s netpool.Session) (netpool.Session, error) {
	return &Session{Sess: s, c: c}, nil
}
func (c *Conn) Setup(_ context.Context) error {
	c.wloop = make(chan *Session)
	c.rloop = make(chan *Session)
	c.Reader = bufio.NewReader(c.Conn)
	go func() { // write request loop
		for {
			select {
			case s, ok := <-c.wloop:
				if !ok {
					// no more request are going out
					close(c.rloop)
					break
				}
				if c.werr != nil {
					s.doneCh <- fmt.Errorf("connection broken, previous err:%w", c.werr)
					return
				}
				err := s.writeRequest(s.ctx)
				if err != nil {
					c.werr = err
					s.Release(true)
					s.doneCh <- err
					return
				}
				c.rloop <- s
			}
		}
	}()
	go func() {
		for {
			select {
			case s, ok := <-c.rloop:
				if !ok {
					// no more responses are coming in
					return
				}
				if c.rerr != nil {
					s.doneCh <- fmt.Errorf("connection broken, previous err:%w", c.rerr)
					return
				}
				err := s.readResponse(s.ctx)
				if err != nil {
					c.rerr = err
					s.Release(true)
				}
				s.doneCh <- err
			}
		}
	}()
	return nil
}

// mimic stdlib behavior
func expectContentLength(r *http.PreparedRequest) bool {
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

var poolHeaderWriter = sync.Pool{New: func() interface{} { return &bufio.Writer{} }}

// writeHeader writes the status and header part of an http 1.1 request
// e.g.:
//
//	GET / HTTP/1.1\r\n
//	Host: www.google.com\r\n
//	X-Xx-Yy: cccccc\r\n
//	\r\n
func writeHeader(c net.Conn, r *http.PreparedRequest) error {
	header := poolHeaderWriter.Get().(*bufio.Writer)
	defer poolHeaderWriter.Put(header)
	defer header.Reset(nil)
	header.Reset(c) // default bufsize is 4096

	header.WriteString(r.Method)
	header.WriteByte(' ')
	header.WriteString(r.U.RequestURI())
	header.WriteString(" HTTP/1.1\r\nHost: ")
	header.WriteString(r.HeaderHost)
	if r.ContentLength > 0 {
		header.WriteString("\r\nContent-Length: ")
		header.WriteString(strconv.FormatInt(r.ContentLength, 10))
		header.WriteString("\r\n")
	} else if r.ContentLength == 0 && expectContentLength(r) {
		header.WriteString("\r\nContent-Length: 0\r\n")
	} else {
		header.WriteString("\r\n")
	}

	for k, v := range r.Header {
		for _, v := range v {
			header.WriteString(k)
			header.WriteString(": ")
			header.WriteString(v)
			header.WriteString("\r\n")
		}
	}
	header.WriteString("\r\n")
	r.Written = true
	return header.Flush()
}

// readHeader first read headers into resp.Header, and process the response according to RFC.
// resp.Header will be kept as-is.
func readHeader(_ context.Context, r *bufio.Reader, req *http.PreparedRequest, resp *http.Response) (close bool, err error) {
	if err := readRawHeader(textproto.NewReader(r), resp); err != nil {
		return true, err
	}
	if !strings.HasPrefix(resp.Proto, "HTTP/") || len(resp.Proto) != len("HTTP/X.Y") || resp.Proto[6] != '.' {
		return true, errors.New("malformed HTTP version")
	}
	httpver := int(resp.Proto[5]-'0')<<4 | int(resp.Proto[7]-'0')

	// the header key was canonicalized while reading from the stream
	contentLens := resp.Header["Content-Length"]

	// RFC 9112 Section 6.3
	// 8. Otherwise, this is a response message without a declared message body length,
	// so the message body length is determined by the number of octets received prior
	// to the server closing the connection.
	resp.ContentLength = -1
	if len(contentLens) > 0 {
		// RFC 9112 Section 6.3
		// 5. If a message is received without Transfer-Encoding and with an invalid
		// Content-Length header field, then the message framing is invalid and
		// the recipient MUST treat it as an unrecoverable error, unless
		// the field value can be successfully parsed as a comma-separated list
		// (Section 5.6.1 of [HTTP]), all values in the list are valid, and all values
		// in the list are the same (in which case, the message is processed with
		// that single value used as the Content-Length field value).
		//
		// Also in RFC 7230 Section 3.3.2
		first := walkReverse(contentLens[0], nil)
		for i := 0; i < len(contentLens); i++ {
			walkReverse(contentLens[i], func(s string) bool {
				if s == "" {
					err = errors.New("http: empty Content-Length value")
				} else if first != s {
					err = fmt.Errorf("http: message cannot contain multiple Content-Length headers; got %q", contentLens)
				}
				return true
			})
		}
		if err != nil {
			return true, err
		}

		// Logic based on Content-Length
		var n uint64
		n, err = strconv.ParseUint(first, 10, 63)
		if err != nil {
			err = errors.New("invalid content-length response header: " + contentLens[0])
			return
		}
		resp.ContentLength = int64(n)
	}

	transferEnc := resp.Header["Transfer-Encoding"]
	if len(transferEnc) > 0 {
		// RFC 9110 Section 5.3
		//  A recipient MAY combine multiple field lines within a field section
		// that have the same field name into one field line, without changing
		// the semantics of the message, by appending each subsequent field line value
		// to the initial field line value in order, separated by a comma (",")
		// and optional whitespace (OWS, defined in Section 5.6.3).
		// ...
		//  This means that, aside from the well-known exception noted below,
		// a sender MUST NOT generate multiple field lines with the same name
		// in a message (whether in the headers or trailers) or append a field line
		// when a field line of the same name already exists in the message, unless
		// that field's definition allows multiple field line values to be recombined
		// as a comma-separated list (i.e., at least one alternative of the field's definition
		// allows a comma-separated list, such as an ABNF rule of #(values) defined in Section 5.6.1).
		//
		// RFC 9112 Section 6.1
		//  Transfer-Encoding = #transfer-coding
		//  transfer-coding    = token *( OWS ";" OWS transfer-parameter )
		resp.TransferEncoding = textproto.TrimString(transferEnc[0])
		for i := 1; i < len(transferEnc); i++ {
			resp.TransferEncoding += ", "
			resp.TransferEncoding += textproto.TrimString(transferEnc[i])
		}
		if resp.TransferEncoding == "" {
			// TODO: audit(warn), empty Transfer-Encoding header
		}
		if httpver < 11 {
			// TODO: audit, Transfer-Encoding header present in HTTP/1.0
			resp.TransferEncoding = ""
		}
	}

	// RFC 9112 Section 6.3

	// 1. Any response to a HEAD request and any response with a 1xx (Informational),
	// 204 (No Content), or 304 (Not Modified) status code is always terminated by
	// the first empty line after the header fields, regardless of the header fields
	// present in the message, and thus cannot contain a message body or trailer section.
	if req.Method == "HEAD" || resp.StatusCode/100 == 1 || resp.StatusCode == 204 || resp.StatusCode == 304 {
		if resp.ContentLength > 0 {
			// TODO: audit
		}
		resp.ContentLength = 0 // not -1, which means "unknwon"
	}

	// 2. Any 2xx (Successful) response to a CONNECT request implies that
	// the connection will become a tunnel immediately after the empty line
	// that concludes the header fields. A client MUST ignore any Content-Length
	// or Transfer-Encoding header fields received in such a message.
	//
	// Also in https://www.rfc-editor.org/rfc/rfc9110#section-9.3.6-12
	if req.Method == "CONNECT" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		resp.TransferEncoding = ""
		resp.ContentLength = -1
	}

	// 3. If a message is received with both a Transfer-Encoding and a Content-Length
	// header field, the Transfer-Encoding overrides the Content-Length.
	// Such a message might indicate an attempt to perform request smuggling (Section 11.2)
	// or response splitting (Section 11.1) and ought to be handled as an error.
	//  An intermediary that chooses to forward the message MUST first remove the received
	// Content-Length field and process the Transfer-Encoding (as described below)
	// prior to forwarding the message downstream.
	if resp.TransferEncoding != "" {
		resp.ContentLength = -1

		// 4.If a Transfer-Encoding header field is present ...
		// ...
		// ... and the chunked transfer coding is not the final encoding,
		// the message body length is determined by reading the connection
		// until it is closed by the server.
		if walkReverse(resp.TransferEncoding, nil) != "chunked" {
			close = true
		}
	} else if resp.ContentLength == -1 {
		close = true
	}

	// 5. ...
	// see above

	/// END RFC 9112 Secion 6.3

	if resp.Header.Get("Connection") == "close" || req.Header.Get("Connection") == "close" {
		close = true
	}
	return
}

func readRawHeader(tp *textproto.Reader, resp *http.Response) error {
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
