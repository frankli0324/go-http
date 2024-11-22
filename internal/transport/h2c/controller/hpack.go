package controller

import (
	"bytes"
	"errors"
	"sync"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

type hpackMixin struct {
	c      *Controller
	hpEnc  *hpack.Encoder
	lastSz uint32

	wBuf *bytes.Buffer

	mu sync.Mutex
}

func (h *hpackMixin) init(c *Controller) {
	h.wBuf = &bytes.Buffer{}
	h.hpEnc = hpack.NewEncoder(h.wBuf)
	h.lastSz = h.hpEnc.MaxDynamicTableSize()
	h.c = c
}

// EncodeHeaders encodes HEADERS frame BlockFragment
func (h *hpackMixin) EncodeHeaders(enumHeaders func(func(k, v string)), do func(data []byte) error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.wBuf.Reset()

	total := uint32(0)
	enumHeaders(func(name, value string) {
		f := hpack.HeaderField{Name: name, Value: value}
		total += f.Size()
		// if total > settings.max header size { error }
	})
	maxWriteHeaderListSize, done1 := h.c.UsePeerSetting(http2.SettingMaxHeaderListSize)
	defer done1()
	if total > maxWriteHeaderListSize {
		return errors.New("http2: request header list larger than peer's advertised limit")
	}

	headerTabSize, done2 := h.c.UsePeerSetting(http2.SettingHeaderTableSize)
	defer done2()
	if h.lastSz != headerTabSize {
		h.hpEnc.SetMaxDynamicTableSize(headerTabSize)
		h.lastSz = headerTabSize
	}
	enumHeaders(func(name, value string) {
		h.hpEnc.WriteField(hpack.HeaderField{Name: name, Value: value})
	})
	return do(h.wBuf.Bytes())
}
