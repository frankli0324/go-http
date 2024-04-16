package h2c

import (
	"context"
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

func (s *Stream) writeCtx(ctx context.Context, writeAction func(context.Context) error) error {
	errCh := make(chan error)
	go func() {
		errCh <- writeAction(ctx)
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		return ErrStreamCancelled.Wrap(s.framer.WriteRSTStream(s.streamID, http2.ErrCodeCancel))
	case err := <-errCh:
		select {
		case <-ctx.Done(): // would return early if cancelled, this logic also appears in standard libs
			return ErrStreamCancelled.Wrap(s.framer.WriteRSTStream(s.streamID, http2.ErrCodeCancel))
		default:
			return err
		}
	}
}

func (s *Stream) WriteHeaders(ctx context.Context, enumHeaders func(func(k, v string)), last bool) error {
	data, err := s.Connection.encodeHeaders(enumHeaders)
	if err != nil {
		return err
	}

	s.writeCtx(ctx, func(ctx context.Context) error {
		// below code consults x/net/http2 func (cc *ClientConn) writeHeaders()

		first := true // first frame written (HEADERS is first, then CONTINUATION)
		for len(data) > 0 {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			chunk := data
			if len(chunk) > int(s.settings.MaxWriteFrameSize) {
				chunk = chunk[:int(s.settings.MaxWriteFrameSize)]
			}
			data = data[len(chunk):]
			endHeaders := len(data) == 0
			if first {
				if err := s.framer.WriteHeaders(http2.HeadersFrameParam{
					StreamID:      s.streamID,
					BlockFragment: chunk,
					EndStream:     last,
					EndHeaders:    endHeaders,
				}, false); err != nil {
					return err
				}
				first = false
			} else {
				s.framer.WriteContinuation(s.streamID, endHeaders, chunk, false)
			}
		}
		return s.framer.Flush()
	})
	return nil
}
