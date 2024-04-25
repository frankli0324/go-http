package controller

import (
	"bytes"
	"errors"
	"sync"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

type hpackMixin struct {
	hpEnc *hpack.Encoder

	wBuf *bytes.Buffer

	muWbuf                 sync.Mutex
	maxWriteHeaderListSize uint32
}

func newHpackMixin(c *Controller) *hpackMixin {
	m := &hpackMixin{}
	m.wBuf = &bytes.Buffer{}
	m.hpEnc = hpack.NewEncoder(m.wBuf)
	m.maxWriteHeaderListSize = c.writeSettings.GetSetting(http2.SettingMaxHeaderListSize)

	c.writeSettings.On(http2.SettingHeaderTableSize, func(value uint32) {
		m.muWbuf.Lock()
		m.hpEnc.SetMaxDynamicTableSize(value)
		m.muWbuf.Unlock()
	})
	c.writeSettings.On(http2.SettingMaxHeaderListSize, func(value uint32) {
		m.muWbuf.Lock()
		m.maxWriteHeaderListSize = value // this value is protected by lock, settings is not
		m.muWbuf.Unlock()
	})
	return m
}

// EncodeHeaders encodes HEADERS frame BlockFragment
func (h *hpackMixin) EncodeHeaders(enumHeaders func(func(k, v string))) ([]byte, error) {
	h.muWbuf.Lock()
	defer h.muWbuf.Unlock()
	h.wBuf.Reset()

	total := uint32(0)
	enumHeaders(func(name, value string) {
		f := hpack.HeaderField{Name: name, Value: value}
		total += f.Size()
		// if total > settings.max header size { error }
	})
	if total > h.maxWriteHeaderListSize {
		return nil, errors.New("http2: request header list larger than peer's advertised limit")
	}
	enumHeaders(func(name, value string) {
		h.hpEnc.WriteField(hpack.HeaderField{Name: name, Value: value})
	})
	return h.wBuf.Bytes(), nil
}
