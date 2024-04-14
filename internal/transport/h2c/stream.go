package h2c

import (
	"net"
)

var _ net.Conn = (*Stream)(nil)

type Stream struct {
	*Connection
	streamID uint32
}

// Close implements net.Conn.
func (s *Stream) Close() error {
	panic("unimplemented")
}

// Read implements net.Conn.
func (s *Stream) Read(b []byte) (n int, err error) {
	panic("unimplemented")
}

// Write implements net.Conn.
func (s *Stream) Write(b []byte) (n int, err error) {
	panic("unimplemented")
}
