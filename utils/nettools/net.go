package nettools

import (
	"net"
	"sync"
	"syscall"
	"time"
)

type Mode int

const (
	ModeEpoll Mode = iota
	ModePoll
	ModeSelect
)

var (
	supported = map[Mode]func(cc []net.Conn, cbUnsure, cbOK func(net.Conn), cbErr func(net.Conn, error), timeout time.Duration){}
	picked    func(cc []net.Conn, cbUnsure, cbOK func(net.Conn), cbErr func(net.Conn, error), timeout time.Duration)
)

func GetConnectionForWrite(cc []net.Conn, cbUnsure, cbOK func(net.Conn), cbErr func(net.Conn, error), timeout time.Duration) {
	picked(cc, cbUnsure, cbOK, cbErr, timeout)
}

func init() {
	for _, mode := range []Mode{ModeEpoll, ModePoll, ModeSelect} {
		if supported[mode] != nil {
			picked = supported[mode]
			break
		}
	}
	if picked == nil {
		picked = func(cc []net.Conn, cbUnsure, _ func(net.Conn), _ func(net.Conn, error), timeout time.Duration) {
			for _, c := range cc {
				cbUnsure(c)
			}
		}
	}
}

func controlFDSet(connections []net.Conn, control func([]int)) {
	if len(connections) == 0 {
		return
	}
	cc := mapConnsToFDs(connections)
	releaser := sync.RWMutex{}
	wg := sync.WaitGroup{}

	releaser.Lock()
	fds := make([]int, len(cc))
	errs := make([]error, len(cc))
	for i, s := range cc {
		if s == nil {
			fds[i] = -1
			continue
		}
		wg.Add(1)
		go func(i int, s syscall.RawConn) {
			// It's annoying that golang docs didn't specify whether the
			// control action will be executed if error occurrs
			// however according to the source code errors would only
			// happen before the control action, here's an example on *[net.conn]:
			//
			//  if err := fd.incref(); err != nil {
			//  	return err
			//  }
			//  defer fd.decref()
			//  f(uintptr(fd.Sysfd))
			//  return nil
			if err := s.Control(func(fd uintptr) {
				fds[i] = int(fd)
				wg.Done()
				releaser.RLock()
			}); err != nil {
				wg.Done()
				errs[i] = err
			}
		}(i, s)
	}
	wg.Wait()
	// if err, do sth
	control(fds)
	releaser.Unlock() // release Control
}

func connsToFD(raw net.Conn) syscall.RawConn {
	if t, ok := raw.(interface{ NetConn() net.Conn }); ok {
		// is *tls.Conn or polyfilled TLS Connection
		raw = t.NetConn()
	}
	if c, ok := raw.(syscall.Conn); ok {
		if c, err := c.SyscallConn(); err == nil {
			return c
		}
	}
	return nil
}

func mapConnsToFDs(cc []net.Conn) []syscall.RawConn {
	rc := make([]syscall.RawConn, len(cc))
	for i, raw := range cc {
		rc[i] = connsToFD(raw)
	}
	return rc
}
