package controller

import (
	"errors"
	"io"
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
	conn.flowControlMixin.init(conn)
	conn.on[http2.FrameGoAway] = func(f http2.Frame) {
		frame := f.(*http2.GoAwayFrame)

		conn.doneOnce.Do(func() {
			debug := frame.DebugData()
			reason := &ReasonGoAway{
				code:   frame.ErrCode,
				debug:  make([]byte, len(debug)),
				remote: true,
			}
			copy(reason.debug, debug)
			conn.doneReason = reason
			close(conn.done)
			if conn.onRemoteGoAway != nil {
				conn.onRemoteGoAway(frame.LastStreamID, frame.ErrCode)
			}
		})
	}
	return conn
}

// Controller holds the same purpose as [golang.org/x/net/http.ClientConn], yet it
// couples with net/http.Transport deeply, so we are re-implementing it.
//
// Controller implements *connection level* flow control, ping/pong,
// settings for both sides, and maintains connection state
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
	flowControlMixin // only for control stream (streamID=0)

	settingsMixin

	on [20]func(http2.Frame) // frame types

	onAfterHandshake []func()
	onRemoteGoAway   func(lastStreamID uint32, errCode http2.ErrCode)
}

// GoAway actively sends GOAWAY to remote peer.
// It must not be called by [Controller] after handshake,
// but should be called by [h2c.Connection] instead
func (c *Controller) GoAway(lastStreamID uint32, code http2.ErrCode) (err error) {
	return c.GoAwayDebug(lastStreamID, code, nil)
}

// GoAwayDebug actively sends GOAWAY to remote peer with debug info.
// It must not be called by [Controller] after handshake,
// but should be called by [h2c.Connection] instead
func (c *Controller) GoAwayDebug(lastStreamID uint32, code http2.ErrCode, debug []byte) (err error) {
	err = ErrMultipleGoAway
	c.doneOnce.Do(func() {
		c.doneReason = &ReasonGoAway{code: code, debug: debug, remote: false}
		close(c.done)
		err = c.WriteGoAway(lastStreamID, code, debug)
		atomic.StoreUint32(&c.closing, 1)
		c.Conn.Close()
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

	if err := c.AdvertiseSelfSettings(c); err != nil {
		return err
	}
	// The server connection preface consists of a potentially empty SETTINGS frame
	// that MUST be the first frame the server sends in the HTTP/2 connection.
	// https://httpwg.org/specs/rfc7540.html#rfc.section.3.5
	f, err := c.ReadFrame()
	if err != nil {
		c.doneOnce.Do(func() {
			c.doneReason = err // TODO: wrap err
			close(c.done)
		})
		return err
	}
	if f.Header().Type == http2.FrameSettings {
		c.on[http2.FrameSettings](f)
	} else {
		_ = c.GoAway(0, http2.ErrCodeProtocol)
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
		f, err := c.ReadFrame()
		if err != nil {
			return err
		}
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

func (c *Controller) OnData(cb func(*http2.DataFrame), onerr func(http2.ErrCode)) {
	c.on[http2.FrameData] = func(f http2.Frame) {
		frame := f.(*http2.DataFrame)
		dl := uint32(len(frame.Data()))
		if !c.inflow.stage(dl) {
			onerr(http2.ErrCodeFlowControl)
		} else {
			cb(frame)
		}
		if inc := c.inflow.grant(uint32(len(frame.Data()))); inc != 0 {
			c.WriteWindowUpdate(0, inc)
		}
	}
}

func (c *Controller) OnHeader(cb func(*http2.MetaHeadersFrame)) {
	c.on[http2.FrameHeaders] = func(f http2.Frame) {
		if f, ok := f.(*http2.MetaHeadersFrame); ok {
			cb(f)
			return
		}
		panic("unexpected frame, framer should return meta headers frame")
	}
}

func (c *Controller) WriteData(streamID uint32, endStream bool, data []byte) (int, error) {
	// wraps framer WriteData for connection level flow control
	if len(data) != 0 {
		bat := c.outflow.take(int32(len(data)))
		data = data[:bat]
	}
	if err := c.framerMixin.WriteData(streamID, endStream, data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (c *Controller) OnRemoteGoAway(cb func(lastStreamID uint32, errCode http2.ErrCode)) {
	c.onRemoteGoAway = cb
}
