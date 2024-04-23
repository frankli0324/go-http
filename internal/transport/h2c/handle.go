package h2c

import (
	"fmt"

	"golang.org/x/net/http2"
)

func (c *Connection) handleMetaHeaders(frame *http2.MetaHeadersFrame) {
	// frame.StreamID
	for _, f := range frame.Fields {
		fmt.Println(f.Name+":", f.Value)
	}
}

func (c *Connection) handleData(frame *http2.DataFrame) {
	fmt.Println(string(frame.Data()))
}
