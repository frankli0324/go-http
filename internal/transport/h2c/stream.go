package h2c

import (
	"net"

	"golang.org/x/net/http2"
)

var _ net.Conn = (*Stream)(nil)

type Stream struct {
	*Connection
	streamID uint32
}

func (s *Stream) ID() uint32 {
	return s.streamID
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

func (s *Stream) Flush() error {
	return s.framer.Flush()
}

func (s *Stream) WriteHeaders(enumHeaders func(func(k, v string)), last bool) error {
	data, err := s.Connection.encodeHeaders(enumHeaders)
	if err != nil {
		return err
	}
	// below code is taken from x/net/http2 func (cc *ClientConn) writeHeaders()

	first := true // first frame written (HEADERS is first, then CONTINUATION)
	for len(data) > 0 {
		chunk := data
		if len(chunk) > int(s.settings.MaxWriteFrameSize) {
			chunk = chunk[:int(s.settings.MaxWriteFrameSize)]
		}
		data = data[len(chunk):]
		endHeaders := len(data) == 0
		if first {
			s.framer.WriteHeaders(http2.HeadersFrameParam{
				StreamID:      s.streamID,
				BlockFragment: chunk,
				EndStream:     last,
				EndHeaders:    endHeaders,
			}, false)
			first = false
		} else {
			s.framer.WriteContinuation(s.streamID, endHeaders, chunk, false)
		}
	}
	s.framer.Flush()
	return nil
}
