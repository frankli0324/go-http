package internal

import (
	"crypto/tls"
	"net"
	"sync"

	"github.com/frankli0324/go-http/internal/transport/h2c"
)

var aliveH2Conns = map[string]*h2c.Connection{}
var muAliveH2Conns = sync.RWMutex{}

func tryDialH2(hostport string) (net.Conn, error) {
	muAliveH2Conns.RLock()
	hc := aliveH2Conns[hostport]
	muAliveH2Conns.RUnlock()
	if hc != nil {
		s, err := hc.Stream()
		if err == nil {
			return s, err
		} else {
			hc.Close()
			muAliveH2Conns.Lock()
			delete(aliveH2Conns, hostport)
			muAliveH2Conns.Unlock()
		} // if h2 create new stream fails, try a new connection
	}
	return nil, net.ErrClosed
}

func negotiateNewH2(hostport string, c *tls.Conn) (net.Conn, error) {
	f := h2c.NewConn(c)
	if err := f.Handshake(); err != nil {
		return nil, err
	}
	muAliveH2Conns.Lock()
	aliveH2Conns[hostport] = f
	muAliveH2Conns.Unlock()
	return f.Stream()
}
