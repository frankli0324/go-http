package netpool

import (
	"context"
	"io"
	"net"
	"sync"
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
	sync.Mutex
	connTicket, idleTicket chan interface{}
	idle                   []*conn
	maxIdleDuration        time.Duration

	dialer func(ctx context.Context) (net.Conn, error)
}

func NewPool(maxIdle, maxConn uint, dialer func(ctx context.Context) (net.Conn, error)) *connPool {
	return &connPool{
		connTicket: make(chan interface{}, maxConn),
		idleTicket: make(chan interface{}, maxIdle),
		dialer:     dialer,
	}
}

func (p *connPool) Connect(ctx context.Context) (io.ReadWriteCloser, error) {
	p.connTicket <- nil
	for {
		select {
		case <-p.idleTicket:
			p.Lock()
			c := p.idle[0]
			p.idle = p.idle[1:]
			p.Unlock()
			if p.maxIdleDuration != 0 && time.Since(c.LastIdle) > p.maxIdleDuration {
				c.Close()
			} else if !c.IsClosed.Load() {
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
	if !c.IsClosed.Load() {
		select {
		case p.idleTicket <- nil:
			p.Lock()
			c.LastIdle = time.Now()
			p.idle = append(p.idle, c)
			p.Unlock()
		default:
			c.Close()
		}
	}
}
