package controller

import (
	"sync"

	"golang.org/x/net/http2"
)

// why on earth is it required to implement flow control in http2???

type inflow struct {
	remaining uint32 // remote should have this many tokens for sending to us
	queued    uint32 // tokens released by upper layer but kept by flow control
}

func (fm *inflow) init(init uint32) {
	fm.remaining = init
}

// received DATA and sending the data into process buffer
func (fm *inflow) stage(sz uint32) bool {
	if fm.remaining < sz {
		return false
	}
	fm.remaining -= sz
	return true
}

// golang/x/net/http2 says so
const inflowMinRefresh = 4 << 10

// RFC 7540 Section 6.9.1.
const inflowMaxWindow = 1<<31 - 1

// return the token to the sender, after buffer is consumed
func (fm *inflow) grant(sz uint32) uint32 {
	fm.queued += sz
	if fm.queued >= inflowMinRefresh {
		windowUpd := fm.queued
		if windowUpd > inflowMaxWindow {
			panic("flow control update exceeds maximum window size")
		}
		fm.queued = 0
		fm.remaining += windowUpd
		return windowUpd
	}
	return 0
}

type outflow struct {
	// RFC rfc7540 6.5.2
	// Values above the maximum flow-control window size of 2^31-1 MUST be treated
	// as a connection error (Section 5.4.1) of type FLOW_CONTROL_ERROR.
	//
	// so we can maintain incoming remains with int32 instead of uint32
	// to track negative values
	remaining int32 // might be negative
	cond      *sync.Cond
	mu        sync.Mutex
}

func (fm *outflow) init(init int32) {
	fm.remaining = init
	fm.cond = sync.NewCond(&fm.mu)
}

func (fm *outflow) take(sz int32) int32 {
	fm.mu.Lock()
	// rfc7540 6.9.2.
	// A change to SETTINGS_INITIAL_WINDOW_SIZE can cause the available space
	// in a flow-control window to become negative. A sender MUST track the
	// negative flow-control window and MUST NOT send new flow-controlled frames
	// until it receives WINDOW_UPDATE frames that cause the flow-control
	// window to become positive.
	for fm.remaining <= 0 {
		fm.cond.Wait()
	}
	got := sz
	if fm.remaining < sz {
		got = fm.remaining
	}
	fm.remaining -= got
	fm.mu.Unlock()
	return got
}

func (fm *outflow) put(sz int32) bool {
	fm.mu.Lock()
	sum := fm.remaining + sz
	// smart overflow detection that works for negative incrs from golang.org/x/net
	if (sum > sz) == (fm.remaining > 0) {
		fm.remaining = sum
		fm.mu.Unlock()
		fm.cond.Broadcast()
		return true
	}
	fm.mu.Unlock()
	return false
}

// flowControlMixin only implements flow control for control frames, that is, streamID=0
type flowControlMixin struct {
	outflow outflow
	inflow  inflow

	OnStreamWindowUpdate func(streamID, incr uint32)
}

func (flw *flowControlMixin) init(c *Controller) {
	// outflow init
	{
		init := c.peerSettings.v[http2.SettingInitialWindowSize] // 65535
		flw.outflow.init(int32(init))
		c.onAfterHandshake = append(c.onAfterHandshake, func() {
			value, done := c.UsePeerSetting(http2.SettingInitialWindowSize)
			if value > inflowMaxWindow {
				c.GoAway(0, http2.ErrCodeFlowControl)
				return
			}
			if value != init {
				// will decrement the tokens if got initial window size smaller than original default
				flw.outflow.put(int32(value) - int32(init))
			}
			done()
		})
		// rfc7540 S 6.9.2.: A SETTINGS frame cannot alter the connection flow-control window.
		// c.settingsMixin.writeSettings.On(http2.SettingInitialWindowSize, func(value uint32) {})
		c.selfSettings.UseSetting(http2.SettingInitialWindowSize)
		c.on[http2.FrameWindowUpdate] = func(f http2.Frame) {
			frame := f.(*http2.WindowUpdateFrame)
			if frame.StreamID != 0 {
				if flw.OnStreamWindowUpdate != nil {
					flw.OnStreamWindowUpdate(frame.StreamID, frame.Increment)
				} else {
					panic("no stream window update dispatcher registered")
				}
			} else if !flw.outflow.put(int32(frame.Increment)) {
				c.GoAway(0 /*TODO: how to get last stream id*/, http2.ErrCodeFlowControl)
			}
		}
	}
	// inflow init
	{
		init := c.selfSettings.v[http2.SettingInitialWindowSize]
		flw.inflow.init(init)
	}
}
