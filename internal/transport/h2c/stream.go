package h2c

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"

	errs "github.com/frankli0324/go-http/internal/transport/h2c/errors"
	"golang.org/x/net/http2"
)

type Stream struct {
	*Connection
	inflow   InflowCtrl
	outflow  OutflowCtrl
	streamID uint32

	// TODO: don't block receiving loop
	chanHeaders chan *http2.MetaHeadersFrame
	respWriter  *io.PipeWriter // http2 frame read loop write data
	respReader  *io.PipeReader // user read data

	rstOnce sync.Once

	doneReason error
	doneOnce   sync.Once

	done chan struct{} // either us or them reset the stream
}

func (s *Stream) Valid() bool {
	if s == nil {
		return false
	}
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

func (s *Stream) ID() uint32 {
	return s.streamID
}

func (s *Stream) Read(b []byte) (n int, err error) {
	return 0, errors.ErrUnsupported
}

func (s *Stream) Write(b []byte) (n int, err error) {
	return 0, errors.ErrUnsupported
}

func (s *Stream) Close() error {
	return s.CloseWithError(nil)
}

func (s *Stream) CloseWithError(err error) error {
	s.doneOnce.Do(func() {
		s.doneReason = err
		close(s.done)
		s.Connection.ReleaseStreamID(s)
		if err != nil {
			s.respWriter.CloseWithError(err)
		}
	})
	return nil
}

func (s *Stream) Reset(code http2.ErrCode, isReceived bool) (err error) {
	s.rstOnce.Do(func() {
		if !isReceived {
			err = s.controller.WriteRSTStream(s.streamID, code)
			s.CloseWithError(errs.ErrStreamResetLocal(s.streamID, code))
		} else {
			s.CloseWithError(errs.ErrStreamResetRemote(s.streamID, code))
		}
	})
	return err
}

// TODO: maybe change this api
func (s *Stream) WriteHeaders(ctx context.Context, enumHeaders func(func(k, v string)), last bool) error {
	return s.controller.EncodeHeaders(enumHeaders, func(data []byte) error {
		// below code consults x/net/http2 func (cc *ClientConn) writeHeaders()

		first := true // first frame written (HEADERS is first, then CONTINUATION)
		for len(data) > 0 {
			select {
			case <-ctx.Done():
				return errs.ErrStreamCancelled(s.streamID)
			default:
			}
			var chunk []byte

			maxWriteFrameSz, done := s.controller.UsePeerSetting(http2.SettingMaxFrameSize)
			endHeaders := len(data) <= int(maxWriteFrameSz)
			if !endHeaders {
				chunk, data = data[:maxWriteFrameSz], data[maxWriteFrameSz:]
			} else {
				chunk, data = data, nil
			}
			var err error
			if first {
				err = s.controller.WriteHeaders(http2.HeadersFrameParam{
					StreamID:      s.streamID,
					BlockFragment: chunk,
					EndStream:     last,
					EndHeaders:    endHeaders,
				})
				first = false
			} else {
				err = s.controller.WriteContinuation(s.streamID, endHeaders, chunk)
			}
			done()
			if err != nil {
				return errs.ErrFramerWrite(s.streamID).Wrap(err)
			}
		}
		return nil
	})
}

// TODO: maybe change this api
func (s *Stream) ReadHeaders(ctx context.Context, headersCb func(k, v string) error) error {
	select {
	case <-ctx.Done():
		return errs.ErrStreamCancelled(s.streamID).Wrap(s.Reset(http2.ErrCodeCancel, false))
	case <-s.done:
		return s.doneReason
	case headers := <-s.chanHeaders:
		for _, kv := range headers.Fields {
			if err := headersCb(kv.Name, kv.Value); err != nil {
				s.Reset(http2.ErrCodeInternal, false)
				return err
			}
		}
	}
	return nil
}

var bodyWriteBuf = (&bufPool{}).init(
	[]int{
		1024,
		1024 * 16, // default max frame size
		1024 * 512},
	[]int{
		1024 * 8,
		1024 * 24,
		math.MaxInt},
)

func (s *Stream) takeOutflow(sz uint32) uint32 {
	s.condOutflow.L.Lock()
	for !s.outflow.Available() || !s.Connection.outflow.Available() {
		s.condOutflow.Wait()
	}
	take1 := s.outflow.Pay(sz)
	take2 := s.Connection.outflow.Pay(take1)
	if take2 < take1 {
		// returning less than had, will not overflow
		s.outflow.Refund(take1 - take2)
	}
	s.condOutflow.L.Unlock()
	return take2
}

func (s *Stream) WriteRequestBody(ctx context.Context, data io.Reader, sz int64, last bool) error {
	read := int64(0)
	maxWriteFrameSz, doneMWFS := s.controller.UsePeerSetting(http2.SettingMaxFrameSize)
	defer doneMWFS()
	bufSz := int(maxWriteFrameSz)
	if sz != -1 {
		bufSz = min(int(bufSz), int(sz))
	}
	buffer, bufIdx := bodyWriteBuf.get(bufSz)
	defer bodyWriteBuf.put(bufIdx, buffer)
	chunk := (*buffer)[:bufSz]
	current := uint32(0)
	readThreshold := uint32(min(bufSz-1024, 512))
	sawEOF := false
	var lastRdErr error
	for {
		select {
		case <-ctx.Done():
			return errs.ErrStreamCancelled(s.streamID)
		default:
		}
		if !sawEOF && current < readThreshold {
			var l int
			l, lastRdErr = data.Read(chunk[current:])
			sawEOF = lastRdErr == io.EOF
			current += uint32(l)
			read += int64(l)
		}
		w := s.takeOutflow(current)
		endStream := last && sawEOF && w == current
		if w > 0 || endStream {
			if err := s.controller.WriteData(s.streamID, endStream, chunk[:w]); err != nil {
				return errs.ErrFramerWrite(s.streamID).Wrap(err)
			}
			copy(chunk[:current-w], chunk[w:current])
			current -= w
		}
		if sz != -1 && read > sz {
			return errs.ErrReqBodyTooLong(s.streamID)
		}
		if endStream {
			if lastRdErr == io.EOF && sz != -1 && read < sz {
				lastRdErr = io.ErrUnexpectedEOF
			}
			if lastRdErr != io.EOF {
				return errs.ErrReqBodyRead(s.streamID).Wrap(lastRdErr)
			}
			return nil // EOF
		}
	}
}

// TODO: maybe change this api
func (s *Stream) ResponseBodyStream(ctx context.Context) io.ReadCloser {
	// TODO: close response reader if error occurrs: remote closed, self cancelled, etc.
	return s.respReader
}
