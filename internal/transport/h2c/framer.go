package h2c

import (
	"sync"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func newFramerMixin(c *Connection) *framerMixin {
	framer := http2.NewFramer(c.Conn, c.Conn)
	framer.SetMaxReadFrameSize(GetMaxFrameSize(c.settings.MaxReadFrameSize))
	framer.ReadMetaHeaders = hpack.NewDecoder(c.settings.HeaderTableSize, nil)
	framer.MaxHeaderListSize = c.settings.MaxReadHeaderListSize

	return &framerMixin{Framer: framer}
}

type framerMixin struct {
	muWrite sync.Mutex
	*http2.Framer
}

func (f *framerMixin) WriteSettings(settings ...http2.Setting) error {
	f.muWrite.Lock()
	err := f.Framer.WriteSettings(settings...)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteHeaders(p http2.HeadersFrameParam) error {
	f.muWrite.Lock()
	err := f.Framer.WriteHeaders(p)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteContinuation(streamID uint32, endHeaders bool, headerBlockFragment []byte) error {
	f.muWrite.Lock()
	err := f.Framer.WriteContinuation(streamID, endHeaders, headerBlockFragment)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteData(streamID uint32, endStream bool, data []byte) error {
	f.muWrite.Lock()
	err := f.Framer.WriteData(streamID, endStream, data)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WritePing(ack bool, data [8]byte) error {
	f.muWrite.Lock()
	err := f.Framer.WritePing(ack, data)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteRSTStream(streamID uint32, code http2.ErrCode) error {
	f.muWrite.Lock()
	err := f.Framer.WriteRSTStream(streamID, code)
	f.muWrite.Unlock()
	return err
}
