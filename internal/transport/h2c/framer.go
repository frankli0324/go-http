package h2c

import (
	"errors"
	"net"
	"sync"
)

type Frame struct {
}

type Framer struct {
	net.Conn
	mu sync.Mutex

	streams map[int]chan *Frame
}

func NewFramer(c net.Conn) *Framer {
	return &Framer{Conn: c, streams: map[int]chan *Frame{}}
}

// Close on *Framer should try to gracefully shutdown the underlying connection asynchronously
func (f *Framer) Close() error {
	panic("unimplemented")
}

// HandshakePriorKnowledge performs PRI handshake on the underlying [net.Conn]
func (f *Framer) HandshakePriorKnowledge() error {
	if _, err := f.Conn.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")); err != nil {
		return err
	}
	panic("unimplemented")
}

// HandshakeUpgrade performs Upgrade handshake on the underlying [net.Conn]
// GET / HTTP/1.1
// Host: example.com
// Connection: Upgrade, HTTP2-Settings
// Upgrade: h2c
// HTTP2-Settings: <base64url encoding of HTTP/2 SETTINGS payload>
func (f *Framer) HandshakeUpgrade(host string) error {
	panic("unimplemented")
}

func (f *Framer) Read([]byte) (int, error) {
	return 0, &net.OpError{
		Op: "read", Err: errors.New("h2c streams can't be read from directly"),
	}
}

func (f *Framer) Write([]byte) (n int, err error) {
	return 0, &net.OpError{
		Op: "read", Err: errors.New("h2c streams can't be written to directly"),
	}
}

func (f *Framer) ReadFrame() *Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	return nil
}

func (f *Framer) WriteFrame(*Frame) {
	f.mu.Lock()
	defer f.mu.Unlock()
}

func (f *Framer) Stream() (net.Conn, error) {
	// streamid := handshake()...
	s := &Stream{f.Conn, f}
	// f.streams[streamid] = s
	return s, nil
}
