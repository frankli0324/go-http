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

func NewPool(maxIdle, maxConn uint, maxIdleDuration time.Duration) (p *Pool) {
	p = &Pool{idleTicket: make(chan *conn, maxIdle), maxIdleDuration: maxIdleDuration}
	if maxConn != 0 {
		p.connTicket = make(chan interface{}, maxConn)
	}
	return
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
			if p.connTicket != nil {
				p.connTicket <- nil
			}
			c, err := dial(ctx)
			return &conn{conn: c, p: p}, err
		}
	}
}
