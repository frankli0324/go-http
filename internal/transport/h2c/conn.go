package h2c

import (
	"errors"
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
		inflow:        &inflow{}, // TODO: support configure custom flow ctrl
		outflow:       &outflow{},
	}
	conn.condOutflow.L = &sync.Mutex{}

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
		if !conn.inflow.CheckAndPay(frame.Length) {
			conn.GoAway(http2.ErrCodeFlowControl)
		}
		data := frame.Data()
		pad := frame.Length - uint32(len(data))
		if pad > 0 {
			conn.inflow.Refund(pad)
		}
		conn.withStream(frame.StreamID, func(active *Stream) error {
			read := 0
			for read != len(data) {
				if n, err := active.respWriter.Write(data); err != nil {
					if inc := conn.inflow.Refund(uint32(len(data) - n)); inc != 0 {
						ctrl.WriteWindowUpdate(0, inc)
					}
					return err
				} else {
					if inc := conn.inflow.Refund(uint32(n)); inc != 0 {
						ctrl.WriteWindowUpdate(0, inc)
					}
					read += n
				}
			}
			if frame.StreamEnded() {
				active.respWriter.Close()
				active.Close()
			}
			return nil
		})
	})
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
	ctrl.OnSettings(func(sf *http2.SettingsFrame) {
		if sf.IsAck() {
			// TODO: activate pending self settings
			return
		}
		done, err := ctrl.UpdatePeerSettings(sf)
		defer done() // holding write lock, can read settings after ack is written and actions done
		if err != nil {
			var cerr http2.ConnectionError
			var code = http2.ErrCodeProtocol
			if errors.As(err, &cerr) {
				code = http2.ErrCode(cerr)
			}
			conn.GoAwayDebug(code, []byte("received invalid settings frame"))
			return
		}

		if iw, ok := sf.Value(http2.SettingInitialWindowSize); ok {
			// rfc9113 6.9.2:
			//
			// In addition to changing the flow-control window for streams that are not yet active,
			// a SETTINGS frame can alter the initial flow-control window size for streams with
			// active flow-control windows (that is, streams in the "open" or "half-closed (remote)" state).
			// When the value of SETTINGS_INITIAL_WINDOW_SIZE changes, a receiver MUST adjust the size of all
			// stream flow-control windows that it maintains by the difference between the new value and the
			// old value.
			conn.muActive.Lock()
			defer conn.muActive.Unlock() // prevent frame writes until after ack is written
			conn.condOutflow.L.Lock()
			for _, stream := range conn.activeStreams {
				stream.outflow.ResetInitialBalance(iw)
			}
			conn.condOutflow.L.Unlock()
			conn.condOutflow.Broadcast()
		}

		_ = ctrl.WriteSettingsAck()
	})

	swnd, done := ctrl.UseSelfSetting(http2.SettingInitialWindowSize)
	conn.inflow.ResetInitialBalance(swnd)
	done()
	// rfc7540 S 6.9.2.: A SETTINGS frame cannot alter the connection flow-control window.
	// so conn.outflow.ResetInitialBalance is called only at initialization
	iw, done := ctrl.UsePeerSetting(http2.SettingInitialWindowSize)
	conn.outflow.ResetInitialBalance(iw)
	done()
	ctrl.OnAfterHandshake(func() {
	})
	ctrl.OnWindowUpdate(func(frame *http2.WindowUpdateFrame) {
		conn.condOutflow.L.Lock()
		defer conn.condOutflow.L.Unlock()
		if frame.StreamID == 0 {
			if !conn.outflow.Refund(frame.Increment) {
				conn.GoAway(http2.ErrCodeFlowControl)
			}
		} else {
			conn.withStream(frame.StreamID, func(s *Stream) error {
				if !s.outflow.Refund(frame.Increment) {
					s.Reset(http2.ErrCodeFlowControl, false)
				}
				return nil
			})
		}
		conn.condOutflow.Broadcast()
	})
	return conn
}

type Connection struct {
	net.Conn
	inflow      InflowCtrl
	outflow     OutflowCtrl
	condOutflow sync.Cond

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
		done:        make(chan struct{}),

		respReader: r, respWriter: w,

		inflow: &inflow{}, outflow: &outflow{},
	}
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
	pwnd, done := s.controller.UsePeerSetting(http2.SettingInitialWindowSize)
	s.outflow.ResetInitialBalance(pwnd)
	done()
	swnd, done := s.controller.UseSelfSetting(http2.SettingInitialWindowSize)
	s.inflow.ResetInitialBalance(swnd) // should be always valid
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

func (c *Connection) GoAwayDebug(code http2.ErrCode, debug []byte) error {
	return c.controller.GoAwayDebug(uint32(atomic.LoadInt32(&c.lastStreamID)), code, debug)
}

// ShouldProcess checks if stream should be retried on a new connection
// if some error occurred
func (c *Connection) ShouldProcess(streamID uint32) bool {
	if c.controller.Valid() == nil {
		return true
	}
	return uint32(atomic.LoadInt32(&c.lastStreamID)) >= streamID
}
