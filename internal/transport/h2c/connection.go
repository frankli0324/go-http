package h2c

import (
	"crypto/rand"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
)

func NewConn(c net.Conn) *Connection {
	settings := NewSettings()
	conn := &Connection{
		Conn:     c,
		streams:  map[int32]chan *http2.Frame{},
		settings: settings,

		pingFut:      map[[8]byte]chan interface{}{},
		lastStreamID: -1, // add 2 each time
		hpackMixin:   newHpackMixin(settings),
		framer:       newFramerMixin(c, settings),
	}

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

	streams  map[int32]chan *http2.Frame
	settings *ClientSettings

	pingFut map[[8]byte]chan interface{}
	muPing  sync.RWMutex

	lastStreamID int32
}

// Handshake performs PRI handshake on the underlying [net.Conn]
func (c *Connection) Handshake() error {
	if _, err := io.WriteString(c.Conn, http2.ClientPreface); err != nil {
		return err
	}
	if err := c.framer.WriteSettings(); err != nil {
		return err
	}
	// The server connection preface consists of a potentially empty SETTINGS frame
	// that MUST be the first frame the server sends in the HTTP/2 connection.
	// https://httpwg.org/specs/rfc7540.html#rfc.section.3.5
	f, err := c.framer.ReadFrame()
	if err != nil {
		return err
	}
	if s, ok := f.(*http2.SettingsFrame); ok {
		c.handleSettings(s)
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

// Ping could fail due to unstable connection when the server doesn't acknoledge it in 10 seconds,
// try not make connection state change decisions based on the Ping results.
// This is mostly used for debugging and keeping connection alive.
// Ping shouldn't be called rapidly or a large number of channels would be created.
func (c *Connection) Ping() error {
	data, res := [8]byte{}, make(chan interface{})
	if _, err := rand.Read(data[:]); err != nil {
		return err
	}
	c.muPing.Lock()
	c.pingFut[data] = res
	c.muPing.Unlock()
	defer func() {
		c.muPing.Lock()
		delete(c.pingFut, data)
		c.muPing.Unlock()
		close(res)
	}()

	if err := c.framer.WritePing(false, data); err != nil {
		return err
	}
	select {
	case <-time.After(10 * time.Second):
		return errors.New("ping timeout 10 seconds")
	case <-res:
		return nil
	}
}

func (c *Connection) consumer() error {
	for atomic.LoadUint32(&c.closing) == 0 {
		f, err := c.framer.ReadFrame()
		if err != nil {
			return err
		}
		switch frame := f.(type) {
		case *http2.PingFrame:
			c.handlePing(frame)
		case *http2.SettingsFrame:
			c.handleSettings(frame)
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
