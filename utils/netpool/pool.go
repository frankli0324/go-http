package netpool

import (
	"context"
	"io"
	"net"
	"time"
)

type Conn interface {
	io.ReadWriteCloser
	Release()
	Raw() net.Conn
}

type Pool struct {
	connTicket      chan interface{}
	idleTicket      chan *conn
	maxIdleDuration time.Duration
}

func NewPool(maxIdle, maxConn uint) *Pool {
	return &Pool{
		connTicket: make(chan interface{}, maxConn),
		idleTicket: make(chan *conn, maxIdle),
	}
}

func (p *Pool) Connect(ctx context.Context, dial func(ctx context.Context) (net.Conn, error)) (Conn, error) {
	for {
		select {
		case c := <-p.idleTicket:
			if p.maxIdleDuration != 0 && time.Since(c.LastIdle) > p.maxIdleDuration {
				c.Close()
			} else if c.Available() {
				return c, nil
			}
		default:
			p.connTicket <- nil
			c, err := dial(ctx)
			return &conn{conn: c, p: p}, err
		}
	}
}
