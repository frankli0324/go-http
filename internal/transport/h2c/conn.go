package h2c

import (
	"net"
	"sync"
	"sync/atomic"

	"github.com/frankli0324/go-http/internal/transport/h2c/controller"
	"golang.org/x/net/http2"
)

func NewConn(c net.Conn) *Connection {
	ctrl := controller.NewController(c)
	conn := &Connection{
		Conn:          c,
		controller:    ctrl,
		lastStreamID:  -1, /* step up by 2 */
		activeStreams: make(map[uint32]*Stream),
	}
	ctrl.OnHeader(func(frame *http2.MetaHeadersFrame) {
		conn.withStream(frame.StreamID, func(active *Stream) error {
			active.chanHeaders <- frame
			if frame.StreamEnded() {
				active.dataWriter.Close() // no body
				active.Close()
			}
			return nil
		})
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
	return conn
}

type Connection struct {
	net.Conn
	controller   *controller.Controller
	lastStreamID int32

	activeStreams map[uint32]*Stream
	muActive      sync.RWMutex
}

func (c *Connection) Handshake() error {
	return c.controller.Handshake()
}

// withStream must be executed only by frame read loop synchronously
func (c *Connection) withStream(streamID uint32, f func(*Stream) error) {
	c.muActive.RLock()
	active := c.activeStreams[streamID]
	c.muActive.RUnlock()
	// active.Lock() // if not synchornous, need lock
	// defer active.Unlock()
	if !active.Valid() {
		c.controller.WriteRSTStream(streamID, http2.ErrCodeStreamClosed)
		// if err goaway
	} else if err := f(active); err != nil {
		c.controller.WriteRSTStream(streamID, http2.ErrCodeProtocol)
	}
}

func (c *Connection) Stream() (*Stream, error) {
	if err := c.controller.Valid(); err != nil {
		return nil, err
	}
	streamID := uint32(atomic.AddInt32(&c.lastStreamID, 2))

	// TODO: check streamid inside 2^31 and match connection setting
	s := newStream(c.controller, streamID, func() {
		c.muActive.Lock()
		delete(c.activeStreams, streamID)
		c.muActive.Unlock()
	})
	c.muActive.Lock()
	c.activeStreams[streamID] = s
	c.muActive.Unlock()
	// TODO: rfc7540 6.9.2
	// In addition to changing the flow-control window for streams that are not yet active,
	// a SETTINGS frame can alter the initial flow-control window size for streams with
	// active flow-control windows (that is, streams in the "open" or "half-closed (remote)" state).
	// When the value of SETTINGS_INITIAL_WINDOW_SIZE changes, a receiver MUST adjust the size of all
	// stream flow-control windows that it maintains by the difference between the new value and the
	// old value.
	return s, nil
}

func (c *Connection) Close() error {
	// never close unless goaway
	return nil
}

func (c *Connection) GoAway() error {
	return c.controller.GoAway(uint32(c.lastStreamID), http2.ErrCodeNo)
}
