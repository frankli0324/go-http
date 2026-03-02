package netpool

import (
	"errors"
	"sync/atomic"
	"time"
)

type state struct {
	conn     Conn
	p        *Pool
	IsClosed uint32
	LastIdle time.Time
}

func (c *state) Available() bool {
	return atomic.LoadUint32(&c.IsClosed) == 0
}

func (c *state) Release(close bool) (reused bool, err error) {
	if !c.Available() {
		return false, errors.New("called Release on closed connection")
	}
	if close {
		return false, c.Close()
	}
	c.LastIdle = time.Now()
	select {
	case c.p.idleTicket <- c:
		reused = true
	default:
		err = c.Close()
	}
	return
}

func (c *state) Close() error {
	err := c.conn.Close()
	atomic.StoreUint32(&c.IsClosed, 1)
	if c.p.connTicket != nil {
		<-c.p.connTicket
	}
	return err
}
