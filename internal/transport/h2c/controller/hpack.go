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

func (h *hpackMixin) init(c *Controller) {
	h.wBuf = &bytes.Buffer{}
	h.hpEnc = hpack.NewEncoder(h.wBuf)
	h.maxWriteHeaderListSize = c.writeSettings.GetSetting(http2.SettingMaxHeaderListSize)

	c.writeSettings.On(http2.SettingHeaderTableSize, func(value uint32) {
		h.muWbuf.Lock()
		h.hpEnc.SetMaxDynamicTableSize(value)
		h.muWbuf.Unlock()
	})
	c.writeSettings.On(http2.SettingMaxHeaderListSize, func(value uint32) {
		h.muWbuf.Lock()
		h.maxWriteHeaderListSize = value // this value is protected by lock, settings is not
		h.muWbuf.Unlock()
	})
}

// EncodeHeaders encodes HEADERS frame BlockFragment
func (h *hpackMixin) EncodeHeaders(enumHeaders func(func(k, v string))) ([]byte, func(), error) {
	h.muWbuf.Lock()
	h.wBuf.Reset()

	total := uint32(0)
	enumHeaders(func(name, value string) {
		f := hpack.HeaderField{Name: name, Value: value}
		total += f.Size()
		// if total > settings.max header size { error }
	})
	if total > h.maxWriteHeaderListSize {
		return nil, nil, errors.New("http2: request header list larger than peer's advertised limit")
	}
	enumHeaders(func(name, value string) {
		h.hpEnc.WriteField(hpack.HeaderField{Name: name, Value: value})
	})
	return h.wBuf.Bytes(), h.muWbuf.Unlock, nil
}
