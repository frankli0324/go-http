package h2c

import (
	"errors"
	"io"
	"net"
	"sync/atomic"

	"golang.org/x/net/http2"
)

func NewConn(c net.Conn) *Connection {
	conn := &Connection{
		Conn:         c,
		streams:      map[int32]chan *http2.Frame{},
		lastStreamID: -1, // add 2 each time
	}
	conn.settings = newSettings(conn)
	conn.hpackMixin = newHpackMixin(conn)
	conn.framer = newFramerMixin(conn)
	conn.pingMixin = newPingMixin(conn)
	return conn
}

// Connection holds the same purpose as [golang.org/x/net/http.ClientConn], yet it
// couples with net/http.Transport deeply, so we are re-implementing it.
type Connection struct {
	net.Conn

	// closing is a boolean value that instructs the consumer to stop,
	// must be read from and written to atomically
	closing uint32

	framer *framerMixin
	*hpackMixin
	*pingMixin

	streams  map[int32]chan *http2.Frame
	settings *ClientSettings

	on [20]func(http2.Frame) // frame types

	lastStreamID int32
}

// Handshake performs PRI handshake on the underlying [net.Conn]
func (c *Connection) Handshake() error {
	if _, err := io.WriteString(c.Conn, http2.ClientPreface); err != nil {
		return err
	}
	if err := c.framer.WriteSettings(
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: c.settings.MaxReadFrameSize},
		http2.Setting{ID: http2.SettingMaxHeaderListSize, Val: c.settings.MaxReadHeaderListSize},
	); err != nil {
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
func (c *Connection) Upgrade(host string) error {
	panic("unimplemented")
}

// Close on *Framer should try to gracefully shutdown the underlying connection asynchronously
func (c *Connection) Close() error {
	panic("unimplemented")
}

func (c *Connection) consumer() error {
	for atomic.LoadUint32(&c.closing) == 0 {
		f, err := c.framer.ReadFrame()
		if err != nil {
			return err
		}
		if on := c.on[f.Header().Type]; on != nil {
			on(f)
		}
		switch frame := f.(type) {
		case *http2.DataFrame:
			c.handleData(frame)
		case *http2.HeadersFrame:
			// make sure framer.ReadMetaHeaders is initialized
			return errors.New("unexpected frame, framer should return meta headers frame")
		case *http2.MetaHeadersFrame:
			c.handleMetaHeaders(frame)
		}
	}
	return nil
}

func (c *Connection) Stream() (net.Conn, error) {
	// streamid := handshake()...
	streamID := atomic.AddInt32(&c.lastStreamID, 2)

	// TODO: check streamid inside 2^31 and match connection settings

	s := &Stream{c, uint32(streamID)}
	// f.streams[streamid] = s
	return s, nil
}

func (c *Connection) Read([]byte) (int, error) {
	return 0, &net.OpError{
		Op: "read", Err: errors.New("h2c connections can't be read from directly"),
	}
}

func (c *Connection) Write([]byte) (n int, err error) {
	return 0, &net.OpError{
		Op: "read", Err: errors.New("h2c connections can't be written to directly"),
	}
}
