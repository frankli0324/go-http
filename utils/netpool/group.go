package netpool

import (
	"context"
	"io"
	"net"
	"sync"
)

type ConnRequest struct {
	Key  interface{}
	Dial func(ctx context.Context) (net.Conn, error)
}

type connGroup struct {
	sync.RWMutex
	pools map[interface{}]*connPool

	maxConnsPerHost, maxIdlePerHost uint
}

func NewGroup(maxConnsPerHost, maxIdlePerHost uint) *connGroup {
	return &connGroup{
		pools:           map[interface{}]*connPool{},
		maxConnsPerHost: maxConnsPerHost, maxIdlePerHost: maxIdlePerHost,
	}
}

func (g *connGroup) Connect(ctx context.Context, req ConnRequest) (io.ReadWriteCloser, error) {
	g.RLock()
	p, ok := g.pools[req.Key]
	g.RUnlock()
	if ok {
		return p.Connect(ctx)
	}
	g.Lock()
	if p, ok = g.pools[req.Key]; !ok {
		p = NewPool(g.maxIdlePerHost, g.maxConnsPerHost, req.Dial)
		g.pools[req.Key] = p
	}
	g.Unlock()
	return p.Connect(ctx)
}
