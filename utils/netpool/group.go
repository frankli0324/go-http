package netpool

import (
	"context"
	"net"
	"sync"
)

type PoolGroup struct {
	sync.RWMutex
	pools map[interface{}]*Pool

	maxConnsPerHost, maxIdlePerHost uint
}

func NewGroup(maxConnsPerHost, maxIdlePerHost uint) *PoolGroup {
	return &PoolGroup{
		pools:           map[interface{}]*Pool{},
		maxConnsPerHost: maxConnsPerHost, maxIdlePerHost: maxIdlePerHost,
	}
}

func (g *PoolGroup) Connect(ctx context.Context, key interface{}, dial func(ctx context.Context) (net.Conn, error)) (Conn, error) {
	g.RLock()
	p, ok := g.pools[key]
	g.RUnlock()
	if ok {
		return p.Connect(ctx, dial)
	}
	g.Lock()
	if p, ok = g.pools[key]; !ok {
		p = NewPool(g.maxIdlePerHost, g.maxConnsPerHost)
		g.pools[key] = p
	}
	g.Unlock()
	return p.Connect(ctx, dial)
}
