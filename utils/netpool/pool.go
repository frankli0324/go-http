package netpool

import (
	"context"
	"time"
)

type Conn interface {
	Setup(context.Context) error
	Session(context.Context, Session) (Session, error)
	Close() error // called by netpool
}

type Session interface {
	Release(close bool) (reused bool, err error)
}

type Pool struct {
	connTicket      chan interface{}
	idleTicket      chan *state
	maxIdleDuration time.Duration
}

func NewPool(maxIdle, maxConn uint, maxIdleDuration time.Duration) (p *Pool) {
	p = &Pool{idleTicket: make(chan *state, maxIdle), maxIdleDuration: maxIdleDuration}
	if maxConn != 0 {
		p.connTicket = make(chan interface{}, maxConn)
	}
	return
}

func (p *Pool) tryGetConn() (got *state, ok bool) { // true for got conn, false for need new connection
	for {
		select {
		case c := <-p.idleTicket:
			if p.maxIdleDuration != 0 && time.Since(c.LastIdle) > p.maxIdleDuration {
				c.Close()
			} else if c.Available() {
				return c, true
			}
			continue
		default:
		}
		if p.connTicket != nil {
			select {
			case c := <-p.idleTicket:
				if p.maxIdleDuration != 0 && time.Since(c.LastIdle) > p.maxIdleDuration {
					c.Close()
				} else if c.Available() {
					return c, true
				}
				continue
			case p.connTicket <- nil:
				return nil, false
			}
		}
		return
	}
}

func (p *Pool) Connect(ctx context.Context, dial func(ctx context.Context) (Conn, error)) (Session, error) {
	if got, ok := p.tryGetConn(); ok {
		return got.conn.Session(ctx, got)
	}
	c, err := dial(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.Setup(ctx); err != nil {
		return nil, err
	}
	return c.Session(ctx, &state{conn: c, p: p})
}
