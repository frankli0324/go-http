package h2c

import (
	"golang.org/x/net/http2"
)

func (c *Connection) handlePing(frame *http2.PingFrame) {
	if !frame.IsAck() {
		if err := c.framer.WritePing(true, frame.Data); err != nil {
			// close connection
		}
	}
	c.pingmu.RLock()
	if v, ok := c.pinglog[frame.Data]; ok {
		v <- nil // make sure this is inside critical zone
		// or write after close may happen
	}
	// else: warn that server acked to an unknown ping packet
	c.pingmu.RUnlock()
}

func (c *Connection) handleSettings(frame *http2.SettingsFrame) {
	// if frame.IsAck() {
	// 	frame
	// }
	frame.ForeachSetting(func(s http2.Setting) error {
		// c.settings[s.ID] = s.Val
		return nil
	})
}

func (c *Connection) handleData(frame *http2.DataFrame) {
}
