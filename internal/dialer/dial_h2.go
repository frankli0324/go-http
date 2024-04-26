package dialer

import (
	"io"
	"net"

	"github.com/frankli0324/go-http/internal/transport/h2c"
	"github.com/frankli0324/go-http/utils/netpool"
)

// helper
type wStream h2c.Stream

func (s *wStream) Raw() net.Conn {
	return (*h2c.Stream)(s)
}

func tryDialH2Stream(re io.ReadWriteCloser) (io.ReadWriteCloser, error) {
	if conn, ok := re.(netpool.Conn); ok {
		if t, ok := conn.Raw().(*h2c.Connection); ok {
			// is *tls.Conn or polyfilled TLS Connection
			c, err := t.Stream()
			conn.Release()
			return (*wStream)(c), err
		}
	}
	return re, nil
}
