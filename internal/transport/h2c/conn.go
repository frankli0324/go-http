package h2c

import (
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/frankli0324/go-http/internal/transport/h2c/controller"
	"golang.org/x/net/http2"
)

func NewConnection(c net.Conn) *Connection {
	ctrl := controller.NewController(c)
	conn := &Connection{
		Conn:          c,
		controller:    ctrl,
		lastStreamID:  -1, /* step up by 2 */
		activeStreams: make(map[uint32]*Stream),
	}
	conn.condActive = sync.NewCond(&conn.muActive)
	ctrl.OnHeader(func(frame *http2.MetaHeadersFrame) {
		conn.withStream(frame.StreamID, func(active *Stream) error {
			active.chanHeaders <- frame
			if frame.StreamEnded() {
				active.respWriter.Close() // no body
				active.Close()
			}
			return nil
		})
	})
	ctrl.OnData(func(frame *http2.DataFrame) {
		conn.withStream(frame.StreamID, func(active *Stream) error {
			ctrl.WriteWindowUpdate(frame.StreamID, frame.Length) // TODO: stream level flow control
			if _, err := active.respWriter.Write(frame.Data()); err != nil {
				return err
			}
			if frame.StreamEnded() {
				active.respWriter.Close()
				active.Close()
			}
			return nil
		})
	}, func(ec http2.ErrCode) { conn.GoAway(ec) })
	ctrl.OnStreamReset(func(frame *http2.RSTStreamFrame) {
		if frame.ErrCode != http2.ErrCodeNo {
			conn.withStream(frame.StreamID, func(active *Stream) error {
				return active.Reset(frame.ErrCode, true)
			})
		}
	})
	ctrl.OnRemoteGoAway(func(u uint32, err http2.ErrCode) {
		conn.muActive.Lock()
		for id, stream := range conn.activeStreams {
			if id > u {
				stream.Reset(err, true)
				delete(conn.activeStreams, id)
			}
		}
		conn.muActive.Unlock()
	})
	ctrl.OnStreamWindowUpdate = func(streamID, incr uint32) {
		conn.withStream(streamID, func(s *Stream) error {
			// TODO
			return nil
		})
	}
	return conn
}

type Connection struct {
	net.Conn
	controller   *controller.Controller
	lastStreamID int32

	activeStreams map[uint32]*Stream
	muActive      sync.RWMutex
	condActive    *sync.Cond

	muNewStream sync.Mutex // should be held when assigning a new stream ID
}

func (c *Connection) Handshake() error {
	return c.controller.Handshake()
}

// withStream must be executed only by frame read loop synchronously
func (c *Connection) withStream(streamID uint32, f func(*Stream) error) {
	c.muActive.RLock()
	active := c.activeStreams[streamID]
	c.muActive.RUnlock()

	if !active.Valid() {
		c.controller.WriteRSTStream(streamID, http2.ErrCodeStreamClosed)
	} else if err := f(active); err != nil {
		active.Reset(http2.ErrCodeInternal, false)
	}
}

func (c *Connection) Stream() (*Stream, error) {
	if err := c.controller.Valid(); err != nil {
		return nil, err
	}
	r, w := io.Pipe()
	s := &Stream{
		Connection:  c,
		chanHeaders: make(chan *http2.MetaHeadersFrame),
		done:        make(chan interface{}),
		respReader:  r,
		respWriter:  w,
	}
	// TODO: rfc7540 6.9.2
	// In addition to changing the flow-control window for streams that are not yet active,
	// a SETTINGS frame can alter the initial flow-control window size for streams with
	// active flow-control windows (that is, streams in the "open" or "half-closed (remote)" state).
	// When the value of SETTINGS_INITIAL_WINDOW_SIZE changes, a receiver MUST adjust the size of all
	// stream flow-control windows that it maintains by the difference between the new value and the
	// old value.
	return s, nil
}

func (c *Connection) ReleaseStreamID(s *Stream) {
	c.muActive.Lock()
	delete(c.activeStreams, s.streamID)
	c.muActive.Unlock()
	c.condActive.Signal()
}

func (c *Connection) AssignStreamID(s *Stream) func() {
	c.muActive.Lock()
	maxConcStreams, done := c.controller.UsePeerSetting(http2.SettingMaxConcurrentStreams)
	for len(c.activeStreams) >= int(maxConcStreams) {
		done()
		c.condActive.Wait()
		maxConcStreams, done = c.controller.UsePeerSetting(http2.SettingMaxConcurrentStreams)
	}
	c.muNewStream.Lock()
	s.streamID = uint32(atomic.AddInt32(&c.lastStreamID, 2))
	// TODO: check streamid inside 2^31 and match connection setting
	c.activeStreams[s.streamID] = s
	c.muActive.Unlock()
	done()
	return c.muNewStream.Unlock
}

func (c *Connection) Close() error {
	// never close unless goaway
	return nil
}

func (c *Connection) GoAway(code http2.ErrCode) error {
	return c.controller.GoAway(uint32(atomic.LoadInt32(&c.lastStreamID)), code)
}

// ShouldProcess checks if stream should be retried on a new connection
// if some error occurred
func (c *Connection) ShouldProcess(streamID uint32) bool {
	if c.controller.Valid() == nil {
		return true
	}
	return uint32(atomic.LoadInt32(&c.lastStreamID)) >= streamID
}
