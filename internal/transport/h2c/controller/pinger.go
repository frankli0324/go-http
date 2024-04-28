package controller

import (
	"errors"
	"math/rand"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/net/http2"
)

type pingMixin struct {
	pingFut    map[uint64]chan interface{}
	muPing     sync.RWMutex
	_writePing func(ack bool, data [8]byte) error
}

func (p *pingMixin) init(c *Controller) {
	c._writePing = c.WritePing
	c.pingFut = map[uint64]chan interface{}{}
	c.on[http2.FramePing] = func(frame http2.Frame) {
		pingFrame := frame.(*http2.PingFrame)
		if pingFrame.IsAck() {
			c.muPing.RLock()
			if v, ok := c.pingFut[*(*uint64)(unsafe.Pointer(&pingFrame.Data[0]))]; ok {
				v <- nil
				// make sure this is inside critical zone
				// or write after close may happen
			}
			// else: warn that server acked to an unknown ping packet
			c.muPing.RUnlock()
			return
		}
		if pingFrame.StreamID != 0 {
			// GOAWAY, PROTOCOL_ERROR
			return
		}
		// warn: ack ping failed
		_ = c.WritePing(true, pingFrame.Data)
	}
}

// Ping could fail due to unstable connection when the server doesn't acknoledge it in 10 seconds,
// try not make connection state change decisions based on the Ping results.
// This is mostly used for debugging and keeping connection alive.
// Ping shouldn't be called rapidly or a large number of channels would be created.
func (p *pingMixin) Ping() error {
	data := rand.Uint64()
	bdata, res := *(*[8]byte)(unsafe.Pointer(&data)), make(chan interface{})
	p.muPing.Lock()
	p.pingFut[data] = res
	p.muPing.Unlock()
	defer func() {
		p.muPing.Lock()
		delete(p.pingFut, data)
		p.muPing.Unlock()
		close(res)
	}()

	if err := p._writePing(false, bdata); err != nil {
		return err
	}
	select {
	case <-time.After(10 * time.Second):
		return errors.New("ping timeout 10 seconds")
	case <-res:
		return nil
	}
}
