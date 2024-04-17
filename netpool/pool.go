package netpool

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"time"
)

type releaser struct {
	p *connPool
	*conn
}

func (r releaser) Release() {
	r.p.Release(r.conn)
}

func (r releaser) Close() error {
	return r.conn.Close()
}

func (r releaser) Raw() net.Conn {
	return r.conn.conn
}

type connPool struct {
	connTicket      chan interface{}
	idleTicket      chan *conn
	maxIdleDuration time.Duration

	dialer func(ctx context.Context) (net.Conn, error)
}

func NewPool(maxIdle, maxConn uint, dialer func(ctx context.Context) (net.Conn, error)) *connPool {
	return &connPool{
		connTicket: make(chan interface{}, maxConn),
		idleTicket: make(chan *conn, maxIdle),
		dialer:     dialer,
	}
}

func (p *connPool) Connect(ctx context.Context) (io.ReadWriteCloser, error) {
	p.connTicket <- nil
	for {
		select {
		case c := <-p.idleTicket:
			if p.maxIdleDuration != 0 && time.Since(c.LastIdle) > p.maxIdleDuration {
				c.Close()
			} else if atomic.LoadUint32(&c.IsClosed) == 0 {
				return releaser{p, c}, nil
			}
		default:
			c, err := p.dialer(ctx)
			return releaser{p, &conn{conn: c}}, err
		}
	}
}

func (p *connPool) Release(c *conn) {
	<-p.connTicket
	if atomic.LoadUint32(&c.IsClosed) == 0 {
		c.LastIdle = time.Now()
		select {
		case p.idleTicket <- c:
		default:
			c.Close()
		}
	}
}
