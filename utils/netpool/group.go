package netpool

import (
	"context"
	"net"
	"sync"
	"time"
)

type PoolGroup struct {
	sync.RWMutex
	pools map[interface{}]*Pool

	maxConnsPerHost, maxIdlePerHost uint
	maxIdleDuration                 time.Duration
}

func NewGroup(maxConnsPerHost, maxIdlePerHost uint, maxIdleDuration time.Duration) *PoolGroup {
	return &PoolGroup{
		pools:           map[interface{}]*Pool{},
		maxConnsPerHost: maxConnsPerHost,
		maxIdlePerHost:  maxIdlePerHost,
		maxIdleDuration: maxIdleDuration,
	}
}

func (g *PoolGroup) NewEmpty() *PoolGroup {
	return NewGroup(g.maxConnsPerHost, g.maxIdlePerHost, g.maxIdleDuration)
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
		p = NewPool(g.maxIdlePerHost, g.maxConnsPerHost, g.maxIdleDuration)
		g.pools[key] = p
	}
	g.Unlock()
	return p.Connect(ctx, dial)
}
