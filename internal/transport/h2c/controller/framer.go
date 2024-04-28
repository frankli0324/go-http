package controller

import (
	"sync"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

type framerMixin struct {
	muWrite sync.Mutex
	framer  *http2.Framer
}

func (f *framerMixin) init(c *Controller) {
	framer := http2.NewFramer(c.Conn, c.Conn) // framer already has a layer of buffer
	// framer.SetMaxReadFrameSize(GetMaxFrameSize(c.remoteSettings.MaxReadFrameSize))
	framer.ReadMetaHeaders = hpack.NewDecoder(c.readSettings.GetSetting(http2.SettingHeaderTableSize), nil)
	framer.MaxHeaderListSize = c.readSettings.GetSetting(http2.SettingMaxHeaderListSize)
	f.framer = framer
}

func (f *framerMixin) WriteSettings(settings ...http2.Setting) error {
	f.muWrite.Lock()
	err := f.framer.WriteSettings(settings...)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteSettingsAck() error {
	f.muWrite.Lock()
	err := f.framer.WriteSettingsAck()
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteHeaders(p http2.HeadersFrameParam) error {
	f.muWrite.Lock()
	err := f.framer.WriteHeaders(p)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteContinuation(streamID uint32, endHeaders bool, headerBlockFragment []byte) error {
	f.muWrite.Lock()
	err := f.framer.WriteContinuation(streamID, endHeaders, headerBlockFragment)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteData(streamID uint32, endStream bool, data []byte) error {
	f.muWrite.Lock()
	err := f.framer.WriteData(streamID, endStream, data)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WritePing(ack bool, data [8]byte) error {
	f.muWrite.Lock()
	err := f.framer.WritePing(ack, data)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteRSTStream(streamID uint32, code http2.ErrCode) error {
	f.muWrite.Lock()
	err := f.framer.WriteRSTStream(streamID, code)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteGoAway(maxStreamID uint32, code http2.ErrCode, debugData []byte) error {
	f.muWrite.Lock()
	err := f.framer.WriteGoAway(maxStreamID, code, debugData)
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteWindowUpdate(streamID, incr uint32) error {
	f.muWrite.Lock()
	err := f.framer.WriteWindowUpdate(streamID, incr)
	f.muWrite.Unlock()
	return err
}
