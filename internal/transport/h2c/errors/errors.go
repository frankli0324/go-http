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

func reg(msg string) func(streamID uint32) StreamError {
	return func(streamID uint32) StreamError { return StreamError{msg, streamID, nil} }
}

var (
	ErrStreamCancelled = reg("stream cancelled by context")
	ErrStreamReset     = reg("stream already been reset")
	ErrReqBodyTooLong  = reg("internal: request body larger than specified content length")
	ErrReqBodyRead     = reg("internal: request body read error")
	ErrFramerWrite     = reg("internal: framer write error")
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
