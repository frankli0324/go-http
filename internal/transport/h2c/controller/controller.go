package controller

import (
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"golang.org/x/net/http2"
)

func NewController(c net.Conn) *Controller {
	conn := &Controller{
		Conn: c,
		done: make(chan struct{}),
	}
	conn.settingsMixin = newSettingsMixin(conn)
	conn.hpackMixin.init(conn)
	conn.framerMixin.init(conn)
	conn.pingMixin.init(conn)
	conn.on[http2.FrameGoAway] = func(frame http2.Frame) {
		conn.doneOnce.Do(func() {
			frame := frame.(*http2.GoAwayFrame)
			debug := frame.DebugData()
			reason := &ReasonGoAway{
				code:   frame.ErrCode,
				debug:  make([]byte, len(debug)),
				remote: true,
			}
			copy(reason.debug, debug)
			conn.doneReason = reason
			close(conn.done)
		})
	}
	return conn
}

// Controller holds the same purpose as [golang.org/x/net/http.ClientConn], yet it
// couples with net/http.Transport deeply, so we are re-implementing it.
type Controller struct {
	net.Conn

	// closing is a boolean value that instructs the consumer to stop,
	// must be read from and written to atomically
	closing uint32

	done       chan struct{}
	doneOnce   sync.Once
	doneReason error

	framerMixin
	hpackMixin
	pingMixin

	settingsMixin

	on [20]func(http2.Frame) // frame types

	onAfterHandshake []func()
}

// GoAway actively sends GOAWAY to remote peer
func (c *Controller) GoAway(lastStreamID uint32, code http2.ErrCode) (err error) {
	return c.GoAwayDebug(lastStreamID, code, nil)
}

// GoAwayDebug actively sends GOAWAY to remote peer with debug info
func (c *Controller) GoAwayDebug(lastStreamID uint32, code http2.ErrCode, debug []byte) (err error) {
	err = ErrMultipleGoAway
	c.doneOnce.Do(func() {
		c.doneReason = &ReasonGoAway{code: code, debug: debug, remote: false}
		close(c.done)
		err = c.WriteGoAway(lastStreamID, code, debug)
		<-c.done
	})
	return
}

// Valid returns error if connection is no longer available
func (c *Controller) Valid() error {
	select {
	case <-c.done:
		if c.doneReason == nil {
			return ErrReasonNil
		}
		return c.doneReason
	default:
	}
	return nil
}

// Handshake performs PRI handshake on the underlying [net.Conn]
func (c *Controller) Handshake() error {
	if _, err := io.WriteString(c.Conn, http2.ClientPreface); err != nil {
		return err
	}

	if err := c.AdvertiseReadSettings(c); err != nil {
		return err
	}
	// The server connection preface consists of a potentially empty SETTINGS frame
	// that MUST be the first frame the server sends in the HTTP/2 connection.
	// https://httpwg.org/specs/rfc7540.html#rfc.section.3.5
	f, err := c.framer.ReadFrame()
	if err != nil {
		return err
	}
	if f.Header().Type == http2.FrameSettings {
		c.on[http2.FrameSettings](f)
	} else {
		// goaway
		return errors.New("connection error, first frame sent by server not settings")
	}
	for _, f := range c.onAfterHandshake {
		f()
	}

	// successful handshake
	go c.consumer()
	return nil
}

// Upgrade performs HTTP1 Upgrade to h2c on the underlying [net.Conn]
// GET / HTTP/1.1
// Host: example.com
// Connection: Upgrade, HTTP2-Settings
// Upgrade: h2c
// HTTP2-Settings: <base64url encoding of HTTP/2 SETTINGS payload>
func (c *Controller) Upgrade(host string) error {
	panic("unimplemented")
}

// Close on *Framer should try to gracefully shutdown the underlying connection asynchronously
func (c *Controller) WaitAndClose() error {
	panic("unimplemented")
}

func (c *Controller) consumer() error {
	for atomic.LoadUint32(&c.closing) == 0 {
		f, err := c.framer.ReadFrame()
		if err != nil {
			return err
		}
		c.WriteWindowUpdate(f.Header().StreamID, f.Header().Length)
		// keep things sending
		// TODO: implement flow control

		if on := c.on[f.Header().Type]; on != nil {
			on(f)
		}
	}
	return nil
}

func (c *Controller) OnStreamReset(cb func(*http2.RSTStreamFrame)) {
	c.on[http2.FrameRSTStream] = func(f http2.Frame) {
		cb(f.(*http2.RSTStreamFrame))
	}
}

func (c *Controller) OnData(cb func(*http2.DataFrame)) {
	c.on[http2.FrameData] = func(f http2.Frame) {
		cb(f.(*http2.DataFrame))
	}
}

func (c *Controller) OnHeader(cb func(*http2.MetaHeadersFrame)) {
	c.on[http2.FrameHeaders] = func(f http2.Frame) {
		if _, ok := f.(*http2.HeadersFrame); ok {
			log.Printf("unexpected frame, framer should return meta headers frame")
			// TODO: GOAWAY
			return
		}
		cb(f.(*http2.MetaHeadersFrame))
	}
}
