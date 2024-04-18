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
	supported             = map[Mode]func(cc []net.Conn, timeout time.Duration) net.Conn{}
	GetConnectionForWrite func(cc []net.Conn, timeout time.Duration) net.Conn
)

func init() {
	for _, mode := range []Mode{ModeEpoll, ModePoll, ModeSelect} {
		if supported[mode] != nil {
			GetConnectionForWrite = supported[mode]
			break
		}
	}
}

func getAllSysRawConns(cc []net.Conn) ([]syscall.RawConn, []net.Conn) {
	rc := make([]syscall.RawConn, len(cc))
	nrc := []net.Conn(nil)
	for i, raw := range cc {
		if t, ok := raw.(interface{ NetConn() net.Conn }); ok {
			// is *tls.Conn or polyfilled TLS Connection
			raw = t.NetConn()
		}
		if c, ok := raw.(syscall.Conn); ok {
			if c, err := c.SyscallConn(); err == nil {
				rc[i] = c
				continue
			}
		}
		nrc = append(nrc, raw)
	}
	return rc, nrc
}

func controlFDSet(canSelect []syscall.RawConn, control func([]int)) {
	if len(canSelect) == 0 {
		return
	}
	releaser := sync.RWMutex{}
	wg := sync.WaitGroup{}

	releaser.Lock()
	fds := make([]int, len(canSelect))
	errs := make([]error, len(canSelect))
	for i, s := range canSelect {
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
