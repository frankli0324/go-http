package h2c

import (
	"context"
	"errors"
	"io"
	"log"
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

	done   chan interface{} // either us or them reset the stream
	doneCB func()
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
		s.doneCB()
	})
	return nil
}

func (s *Stream) Reset(code http2.ErrCode, isReceived bool) (err error) {
	s.rstOnce.Do(func() {
		if !isReceived {
			err = s.controller.WriteRSTStream(s.streamID, code)
		}
		s.CloseWithError(errs.ErrStreamResetRemote)
	})
	return err
}

var errReturnEarly = errors.New("internal: early return due to context cancelled")
var errReqBodyTooLong = errors.New("internal: request body larger than specified content length")

func (s *Stream) writeCtx(ctx context.Context, writeAction func(context.Context) error) error {
	errCh := make(chan error)
	go func() {
		errCh <- writeAction(ctx)
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		return errs.ErrStreamCancelled.Wrap(s.Reset(http2.ErrCodeCancel, false))
	case err := <-errCh:
		if err == errReturnEarly {
			return errs.ErrStreamCancelled.Wrap(s.Reset(http2.ErrCodeCancel, false))
		} else if err == errReqBodyTooLong {
			return errs.ErrStreamCancelled.Wrap(err)
		}
		return err
	}
}

// TODO: maybe change this api
func (s *Stream) WriteHeaders(ctx context.Context, enumHeaders func(func(k, v string)), last bool) error {
	data, unlock, err := s.controller.EncodeHeaders(enumHeaders)
	defer unlock()
	if err != nil {
		return err
	}

	return s.writeCtx(ctx, func(ctx context.Context) error {
		// below code consults x/net/http2 func (cc *ClientConn) writeHeaders()

		first := true // first frame written (HEADERS is first, then CONTINUATION)
		for len(data) > 0 {
			select {
			case <-ctx.Done():
				return errReturnEarly
			default:
			}
			var chunk []byte

			maxWriteFrameSz := int(s.controller.GetPeerSetting(http2.SettingMaxFrameSize))
			endHeaders := len(data) <= maxWriteFrameSz
			if !endHeaders {
				chunk, data = data[:maxWriteFrameSz], data[maxWriteFrameSz:]
			} else {
				chunk, data = data, nil
			}
			if first {
				err := s.controller.WriteHeaders(http2.HeadersFrameParam{
					StreamID:      s.streamID,
					BlockFragment: chunk,
					EndStream:     last,
					EndHeaders:    endHeaders,
				})
				if err != nil {
					return err
				}
				first = false
			} else if err := s.controller.WriteContinuation(s.streamID, endHeaders, chunk); err != nil {
				return err
			}
		}
		return nil
	})
}

// TODO: maybe change this api
func (s *Stream) ReadHeaders(ctx context.Context, headersCb func(k, v string) error) error {
	select {
	case <-ctx.Done():
		return errs.ErrStreamCancelled.Wrap(s.Reset(http2.ErrCodeCancel, false))
	case <-s.done:
		return s.doneReason
	case headers := <-s.chanHeaders:
		for _, kv := range headers.Fields {
			if err := headersCb(kv.Name, kv.Value); err != nil {
				if err := s.Reset(http2.ErrCodeProtocol, false); err != nil {
					log.Printf("reset stream %d error:%v", s.streamID, err)
					// TODO: connection error
				}
				return err
			}
		}
	}
	return nil
}

var bufPoolShort = sync.Pool{New: func() interface{} {
	r := make([]byte, 1024)
	return &r
}}
var bufPoolLong = sync.Pool{New: func() interface{} {
	r := make([]byte, 16384*2)
	return &r
}}

func getBodyWriteBuf(sz int) (b []byte, put func(interface{})) {
	p := &bufPoolShort
	if sz >= 16384 {
		p = &bufPoolLong
	}
	b = *(p.Get().(*[]byte))
	if sz > len(b) {
		b = append(b, make([]byte, sz-len(b))...)
	}
	return b, p.Put
}

func (s *Stream) WriteRequestBody(ctx context.Context, data io.Reader, sz int64, last bool) error {
	return s.writeCtx(ctx, func(ctx context.Context) error {
		read := int64(0)
		maxWriteFrameSz := int(s.controller.GetPeerSetting(http2.SettingMaxFrameSize))
		bufSz := maxWriteFrameSz
		if sz != -1 {
			bufSz = min(bufSz, int(sz))
		}
		chunk, put := getBodyWriteBuf(bufSz)
		defer put(&chunk)
		for {
			select {
			case <-ctx.Done():
				return errReturnEarly
			default:
			}
			l, err := data.Read(chunk)
			endStream := last && err == io.EOF
			if l != 0 || endStream {
				read += int64(l)
				err := s.controller.WriteData(s.streamID, endStream, chunk[:l])
				if err != nil {
					s.Reset(http2.ErrCodeProtocol, false)
					return err
				}
			}
			if sz != -1 && read > sz {
				s.Reset(http2.ErrCodeInternal, false)
				return errReqBodyTooLong
			}
			if err != nil {
				if err == io.EOF && sz != -1 && read < sz {
					err = io.ErrUnexpectedEOF
				}
				if err != io.EOF {
					s.Reset(http2.ErrCodeInternal, false)
					return err
				}
				return nil // EOF
			}
		}
	})
}

// TODO: maybe change this api
func (s *Stream) ResponseBodyStream(ctx context.Context) io.ReadCloser {
	// TODO: close response reader if error occurrs: remote closed, self cancelled, etc.
	return s.respReader
}
