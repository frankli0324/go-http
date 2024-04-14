package h2c

import (
	"fmt"

	"golang.org/x/net/http2"
)

func (c *Connection) handlePing(frame *http2.PingFrame) {
	if frame.IsAck() {
		c.muPing.RLock()
		if v, ok := c.pingFut[frame.Data]; ok {
			v <- nil // make sure this is inside critical zone
			// or write after close may happen
		}
		// else: warn that server acked to an unknown ping packet
		c.muPing.RUnlock()
		return
	}
	if frame.StreamID != 0 {
		// GOAWAY, PROTOCOL_ERROR
		return
	}
	if err := c.framer.WritePing(true, frame.Data); err != nil {
		// warn: ack ping failed
	}
}

func (c *Connection) handleSettings(frame *http2.SettingsFrame) {
	if frame.IsAck() {
		return
	}
	c.settings.UpdateFrom(frame)
}

func (c *Connection) handleMetaHeaders(frame *http2.MetaHeadersFrame) {
	// frame.StreamID
	for _, f := range frame.Fields {
		fmt.Println(f.Name+":", f.Value)
	}
}

func (c *Connection) handleData(frame *http2.DataFrame) {
	fmt.Println(string(frame.Data()))
}
