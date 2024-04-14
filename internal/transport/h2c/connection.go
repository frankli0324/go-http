package h2c

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func NewConn(c net.Conn) *Connection {
	conn := &Connection{
		Conn:     c,
		streams:  map[int32]chan *http2.Frame{},
		settings: map[http2.SettingID]uint32{},
		framer:   http2.NewFramer(c, c),

		pinglog:      map[[8]byte]chan interface{}{},
		lastStreamID: -1, // add 2 each time
	}

	conn.hbuf = &bytes.Buffer{}
	conn.he = hpack.NewEncoder(conn.hbuf)

	return conn
}

type Connection struct {
	net.Conn

	// closing is a boolean value that instructs the consumer to stop,
	// must be read from and written to atomically
	closing uint32

	streams  map[int32]chan *http2.Frame
	settings map[http2.SettingID]uint32
	framer   *http2.Framer
	hd       *hpack.Decoder
	he       *hpack.Encoder
	hbuf     *bytes.Buffer

	pinglog map[[8]byte]chan interface{}
	pingmu  sync.RWMutex

	lastStreamID int32
}

// Handshake performs PRI handshake on the underlying [net.Conn]
func (c *Connection) Handshake() error {
	if _, err := io.WriteString(c.Conn, http2.ClientPreface); err != nil {
		return err
	}
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

func (c *Connection) Ping() error {
	data, res := [8]byte{}, make(chan interface{})
	if _, err := rand.Read(data[:]); err != nil {
		return err
	}
	c.pingmu.Lock()
	c.pinglog[data] = res
	c.pingmu.Unlock()
	if err := c.framer.WritePing(false, data); err != nil {
		return err
	}
	select {
	case <-time.After(10 * time.Second):
		return errors.New("ping timeout 10 seconds")
	case <-res:
		c.pingmu.Lock()
		delete(c.pinglog, data)
		c.pingmu.Unlock()
		close(res)
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
