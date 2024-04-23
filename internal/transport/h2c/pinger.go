package h2c

import (
	"errors"
	"math/rand"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/net/http2"
)

func newPingMixin(c *Connection) *pingMixin {
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
		_ = c.framer.WritePing(true, pingFrame.Data)
	}
	return &pingMixin{
		framer:  c.framer,
		pingFut: map[uint64]chan interface{}{},
	}
}

type pingMixin struct {
	pingFut map[uint64]chan interface{}
	muPing  sync.RWMutex
	framer  *framerMixin
}

// Ping could fail due to unstable connection when the server doesn't acknoledge it in 10 seconds,
// try not make connection state change decisions based on the Ping results.
// This is mostly used for debugging and keeping connection alive.
// Ping shouldn't be called rapidly or a large number of channels would be created.
func (c *pingMixin) Ping() error {
	data := rand.Uint64()
	bdata, res := *(*[8]byte)(unsafe.Pointer(&data)), make(chan interface{})
	c.muPing.Lock()
	c.pingFut[data] = res
	c.muPing.Unlock()
	defer func() {
		c.muPing.Lock()
		delete(c.pingFut, data)
		c.muPing.Unlock()
		close(res)
	}()

	if err := c.framer.WritePing(false, bdata); err != nil {
		return err
	}
	select {
	case <-time.After(10 * time.Second):
		return errors.New("ping timeout 10 seconds")
	case <-res:
		return nil
	}
}
