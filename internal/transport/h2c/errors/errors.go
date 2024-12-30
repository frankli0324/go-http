package errors

import (
	"strconv"

	"golang.org/x/net/http2"
)

type StreamError struct {
	msg      string
	streamID uint32
	error
}

func (e StreamError) Error() string {
	msg := e.msg + " at stream " + strconv.FormatInt(int64(e.streamID), 10)
	if e.error != nil {
		msg += ", error: " + e.error.Error()
	}
	return msg
}

func (e StreamError) Stream(streamID uint32) StreamError {
	return StreamError{e.msg, streamID, e.error}
}

func (e StreamError) Wrap(err error) StreamError {
	if err == nil {
		return e
	}
	return StreamError{e.msg, e.streamID, err}
}

func (e StreamError) Unwrap() error {
	return e.error
}

func (e StreamError) Is(err error) bool {
	if err, ok := err.(StreamError); ok {
		return e.msg == err.msg
	}
	return false
}

var (
	ErrStreamCancelled = StreamError{"stream cancelled by context", 0, nil}
	ErrStreamReset     = StreamError{"stream already been reset", 0, nil}
	ErrReqBodyTooLong  = StreamError{"internal: request body larger than specified content length", 0, nil}
	ErrReqBodyRead     = StreamError{"internal: request body read error", 0, nil}
	ErrFramerWrite     = StreamError{"internal: framer write error", 0, nil}
)

type h2Code http2.ErrCode

func (c h2Code) Error() string {
	return http2.ErrCode(c).String()
}

var (
	ErrStreamResetRemote = func(streamID uint32, code http2.ErrCode) StreamError {
		return StreamError{"remote stream reset", streamID, h2Code(code)}
	}
	ErrStreamResetLocal = func(streamID uint32, code http2.ErrCode) StreamError {
		return StreamError{"local stream reset", streamID, h2Code(code)}
	}
)
