package chunked

import (
	"fmt"
	"io"
	"net/http"
)

// NewChunkedWriter is taken from golang src/net/http/internal/chunked.go
func NewChunkedWriter(w io.Writer) *chunkedWriter {
	return &chunkedWriter{w}
}

type chunkedWriter struct {
	Wire io.Writer
}

func (cw *chunkedWriter) Write(data []byte) (n int, err error) {

	// Don't send 0-length data. It looks like EOF for chunked encoding.
	if len(data) == 0 {
		return 0, nil
	}

	if _, err = fmt.Fprintf(cw.Wire, "%x\r\n", len(data)); err != nil {
		return 0, err
	}
	if n, err = cw.Wire.Write(data); err != nil {
		return
	}
	if n != len(data) {
		err = io.ErrShortWrite
		return
	}
	if _, err = io.WriteString(cw.Wire, "\r\n"); err != nil {
		return
	}
	if f, ok := cw.Wire.(interface{ Flush() error }); ok {
		err = f.Flush()
	}
	return
}

func (cw *chunkedWriter) CloseWithTrailer(trailer http.Header) error {
	n, err := io.WriteString(cw.Wire, "0\r\n\r\n")
	if err == nil && n != 5 {
		return io.ErrShortWrite
	}
	// TODO: write trailer
	return err
}
