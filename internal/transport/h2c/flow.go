package h2c

type InflowCtrl interface {
	ResetInitialBalance(sz uint32) bool
	CheckAndPay(sz uint32) bool
	Refund(sz uint32) uint32
}

type OutflowCtrl interface {
	Available() bool
	Pay(sz uint32) uint32
	Refund(sz uint32) bool
}

// RFC 7540 Section 6.9.1.
const maxFlowControlWindow = 2<<30 - 1

type inflow struct {
	// RFC rfc7540 6.5.2
	// Values above the maximum flow-control window size of 2^31-1 MUST be treated
	// as a connection error (Section 5.4.1) of type FLOW_CONTROL_ERROR.
	remaining, queued uint32
}

func (fm *inflow) ResetInitialBalance(sz uint32) bool {
	// only set once for now, unless we support updating INITIAL_WINDOW_SIZE from our side
	fm.remaining = sz
	return true
}

// received DATA and sending the data into process buffer
func (fm *inflow) CheckAndPay(sz uint32) bool {
	if fm.remaining < sz {
		return false
	}
	fm.remaining -= sz
	return true
}

// golang/x/net/http2 says so
const inflowMinRefresh = 4 << 10

// return the token to the sender, after buffer is consumed
func (fm *inflow) Refund(sz uint32) uint32 {
	fm.queued += sz
	if fm.queued >= inflowMinRefresh {
		windowUpd := fm.queued
		if windowUpd > maxFlowControlWindow {
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
	n int32
}

func (fm *outflow) Available() bool {
	// rfc7540 6.9.2.
	// A change to SETTINGS_INITIAL_WINDOW_SIZE can cause the available space
	// in a flow-control window to become negative. A sender MUST track the
	// negative flow-control window and MUST NOT send new flow-controlled frames
	// until it receives WINDOW_UPDATE frames that cause the flow-control
	// window to become positive.
	return fm.n > 0
}

func (fm *outflow) Pay(sz uint32) uint32 {
	got := sz
	if avail := uint32(fm.n); avail < sz {
		got = avail
	}
	fm.n -= int32(got)
	return got
}

func (fm *outflow) Refund(sz uint32) bool {
	sum := fm.n + int32(sz)
	// smart overflow detection that works for negative incrs from golang.org/x/net
	if (sum > int32(sz)) == (fm.n > 0) {
		fm.n = sum
		return true
	}
	return false
}
