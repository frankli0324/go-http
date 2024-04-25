package h2c

import (
	"net"
	"sync"
	"sync/atomic"

	"github.com/frankli0324/go-http/internal/transport/h2c/controller"
	"golang.org/x/net/http2"
)

type streamMutex struct {
	sync.Mutex
	*Stream
}

func NewConn(c net.Conn) *Connection {
	ctrl := controller.NewController(c)
	conn := &Connection{
		controller:    ctrl,
		lastStreamID:  -1, /* step up by 2 */
		activeStreams: make(map[uint32]*streamMutex),
		goAway:        make(chan struct{}),
	}
	ctrl.OnHeader(func(frame *http2.MetaHeadersFrame) {
		conn.muActive.RLock()
		active := conn.activeStreams[frame.StreamID]
		conn.muActive.RUnlock()
		active.Lock()
		defer active.Unlock()
		if !active.Valid() {
			ctrl.WriteRSTStream(frame.StreamID, http2.ErrCodeStreamClosed)
			// if err goaway
			return
		}
		active.chanHeaders <- frame
		if frame.StreamEnded() {
			active.Close()
		}
	})
	ctrl.OnData(func(frame *http2.DataFrame) {
		conn.withStream(frame.StreamID, func(active *Stream) error {
			if _, err := active.dataWriter.Write(frame.Data()); err != nil {
				return err
			}
			if frame.StreamEnded() {
				active.dataWriter.Close()
				active.Close()
			}
			return nil
		})
	})
	ctrl.OnStreamReset(func(frame *http2.RSTStreamFrame) {
		conn.withStream(frame.StreamID, func(active *Stream) error {
			return active.Reset(frame.ErrCode, true)
		})
	})
	ctrl.OnGoAway(func(frame *http2.GoAwayFrame) {
		conn.goAwayOnce.Do(func() {
			conn.goAwayReason.code = frame.ErrCode
			debug := frame.DebugData()
			conn.goAwayReason.debug = make([]byte, len(debug))
			copy(conn.goAwayReason.debug, debug)
			conn.goAwayReason.remote = true
			close(conn.goAway)
		})
	})
	return conn
}

type Connection struct {
	controller   *controller.Controller
	lastStreamID int32

	activeStreams map[uint32]*streamMutex
	muActive      sync.RWMutex

	goAwayOnce   sync.Once
	goAwayReason struct {
		code   http2.ErrCode
		debug  []byte
		remote bool
		last   uint32
	}
	goAway chan struct{}
}

func (c *Connection) Handshake() error {
	return c.controller.Handshake()
}

func (c *Connection) withStream(streamID uint32, f func(*Stream) error) {
	c.muActive.RLock()
	active := c.activeStreams[streamID]
	c.muActive.RUnlock()
	active.Lock()
	defer active.Unlock()
	if !active.Valid() {
		c.controller.WriteRSTStream(streamID, http2.ErrCodeStreamClosed)
		// if err goaway
	} else if err := f(active.Stream); err != nil {
		c.controller.WriteRSTStream(streamID, http2.ErrCodeProtocol)
	}
}

func (c *Connection) Stream() (net.Conn, error) {
	select {
	case <-c.goAway:
		return nil, http2.GoAwayError{}
	default:
	}
	// streamid := handshake()...
	streamID := uint32(atomic.AddInt32(&c.lastStreamID, 2))

	// TODO: check streamid inside 2^31 and match connection setting
	s := newStream(c.controller, streamID)
	c.activeStreams[streamID] = &streamMutex{Stream: s}
	// f.streams[streamid] = s
	return s, nil
}

func (c *Connection) Close() error {
	return c.controller.Close()
}
