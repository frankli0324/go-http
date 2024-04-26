package controller

import (
	"errors"
	"fmt"

	"golang.org/x/net/http2"
)

var (
	ErrMultipleGoAway = errors.New("connection already seen GOAWAY")
	ErrReasonNil      = errors.New("connection closed without reason, this is unexpected")
)

type ReasonGoAway struct {
	code   http2.ErrCode
	debug  []byte
	remote bool
	last   uint32
}

func (r *ReasonGoAway) Error() string {
	return fmt.Sprintf("GOAWAY seen on connection, err:%s, send by remote peer:%t, last:%d", r.code.String(), r.remote, r.last)
}
