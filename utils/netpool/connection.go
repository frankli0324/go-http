package netpool

import (
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"
)

type conn struct {
	conn     net.Conn
	p        *Pool
	IsClosed uint32
	LastIdle time.Time
}

// Raw implements Conn.
func (c *conn) Raw() net.Conn {
	return c.conn
}

func (c *conn) Release() {
	if c.Available() {
		c.LastIdle = time.Now()
		select {
		case c.p.idleTicket <- c:
		default:
			c.Close()
		}
	}
}

func (c *conn) Available() bool {
	return atomic.LoadUint32(&c.IsClosed) == 0
}

func (c *conn) Write(p []byte) (n int, err error) {
	n, err = c.conn.Write(p)
	if err != nil {
		if err != io.EOF {
			log.Printf("netpool: error on write. %v\n", err)
		}
		c.Close()
	}
	return
}

func (c *conn) Read(p []byte) (n int, err error) {
	nb, err := c.conn.Read(p)
	if err != nil {
		if err != io.EOF {
			log.Printf("netpool: error on read. %v\n", err)
		}
		c.Close()
	}
	return nb, err
}

func (c *conn) Close() error {
	err := c.conn.Close()
	atomic.StoreUint32(&c.IsClosed, 1)
	if c.p.connTicket != nil {
		<-c.p.connTicket
	}
	return err
}
