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
	streamID uint32

	// TODO: don't block receiving loop
	chanHeaders chan *http2.MetaHeadersFrame
	respWriter  *io.PipeWriter // http2 frame read loop write data
	respReader  *io.PipeReader // user read data

	rstOnce sync.Once

	doneReason error
	doneOnce   sync.Once

	done chan interface{} // either us or them reset the stream
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

// HandleWrite handles errors and timeouts from [WriteHeaders] and [WriteRequestBody]
func (s *Stream) HandleWrite(ctx context.Context, writeAction func(context.Context) error) (err error) {
	done := make(chan struct{})
	go func() { err = writeAction(ctx); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		err = errs.ErrStreamCancelled(s.streamID)
	}
	if err == nil {
		return nil
	}
	if err == errs.ErrStreamCancelled(s.streamID) {
		s.Reset(http2.ErrCodeCancel, false)
	} else if errors.Is(err, errs.ErrFramerWrite(s.streamID)) {
		s.Reset(http2.ErrCodeProtocol, false)
	} else if err != nil {
		s.Reset(http2.ErrCodeInternal, false)
	}
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

func (s *Stream) WriteRequestBody(ctx context.Context, data io.Reader, sz int64, last bool) error {
	read := int64(0)
	maxWriteFrameSz, done := s.controller.UsePeerSetting(http2.SettingMaxFrameSize)
	defer done()
	bufSz := int(maxWriteFrameSz)
	if sz != -1 {
		bufSz = min(int(bufSz), int(sz))
	}
	buffer, bufIdx := bodyWriteBuf.get(bufSz)
	defer bodyWriteBuf.put(bufIdx, buffer)
	chunk := (*buffer)[:bufSz]
	current := 0
	readThreshold := min(bufSz-1024, 512)
	sawEOF := false
	for {
		select {
		case <-ctx.Done():
			return errs.ErrStreamCancelled(s.streamID)
		default:
		}
		var err error
		if !sawEOF && current < readThreshold {
			var l int
			l, err = data.Read(chunk[current:])
			sawEOF = err == io.EOF
			current += l
			read += int64(l)
		}
		endStream := last && sawEOF
		if current > 0 || endStream {
			w, err := s.controller.WriteData(s.streamID, endStream, chunk[:current])
			if err != nil {
				return errs.ErrFramerWrite(s.streamID).Wrap(err)
			}
			copy(chunk[:current-w], chunk[w:current])
			current -= w
		}
		if sz != -1 && read > sz {
			return errs.ErrReqBodyTooLong(s.streamID)
		}
		if err != nil {
			if err == io.EOF && sz != -1 && read < sz {
				err = io.ErrUnexpectedEOF
			}
			if err != io.EOF {
				return errs.ErrReqBodyRead(s.streamID).Wrap(err)
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
