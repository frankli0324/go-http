package h2c

import (
	"bufio"
	"net"
	"sync"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func newFramerMixin(c net.Conn, settings *ClientSettings) *framerMixin {
	wbuf := bufio.NewWriter(c)
	framer := http2.NewFramer(wbuf, c)
	framer.SetMaxReadFrameSize(GetMaxFrameSize(settings.MaxReadFrameSize))
	framer.ReadMetaHeaders = hpack.NewDecoder(settings.HeaderTableSize, nil)
	framer.MaxHeaderListSize = settings.MaxReadHeaderListSize

	return &framerMixin{
		wbuf:   wbuf,
		Framer: framer,
	}
}

type framerMixin struct {
	muWrite sync.Mutex
	wbuf    *bufio.Writer
	*http2.Framer
}

func (f *framerMixin) Flush() error {
	f.muWrite.Lock()
	err := f.wbuf.Flush()
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteHeaders(p http2.HeadersFrameParam, flush bool) error {
	f.muWrite.Lock()
	err := f.Framer.WriteHeaders(p)
	if err != nil && flush {
		err = f.wbuf.Flush()
	}
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteContinuation(streamID uint32, endHeaders bool, headerBlockFragment []byte, flush bool) error {
	f.muWrite.Lock()
	err := f.Framer.WriteContinuation(streamID, endHeaders, headerBlockFragment)
	if err != nil && flush {
		err = f.wbuf.Flush()
	}
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WriteData(streamID uint32, endStream bool, data []byte, flush bool) error {
	f.muWrite.Lock()
	err := f.Framer.WriteData(streamID, endStream, data)
	if err != nil && flush {
		err = f.wbuf.Flush()
	}
	f.muWrite.Unlock()
	return err
}

func (f *framerMixin) WritePing(ack bool, data [8]byte) error {
	f.muWrite.Lock()
	err := f.Framer.WritePing(ack, data)
	if err != nil {
		err = f.wbuf.Flush()
	}
	f.muWrite.Unlock()
	return err
}
