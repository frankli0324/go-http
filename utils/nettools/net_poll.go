//go:build darwin || linux
// +build darwin linux

package nettools

import (
	"errors"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

var errBadConnection = errors.New("bad connection")

var _ = func() error { // make sure this executes before func init()
	supported[ModePoll] = pollForWrite
	return nil
}()

func pollForWrite(cc []net.Conn, cbUnsure, cbOK func(net.Conn), cbErr func(net.Conn, error), timeout time.Duration) {
	controlFDSet(cc, func(u []int) {
		s := make([]unix.PollFd, 0, len(u))
		idx := make([]int, 0, len(u))
		for i, v := range u {
			if v == -1 {
				cbUnsure(cc[i])
				continue
			}
			s = append(s, unix.PollFd{Fd: int32(v), Events: unix.POLLOUT | unix.POLLERR | unix.POLLHUP})
			idx = append(idx, i)
		}

		dur := time.Millisecond * 50 // process shouldn't hang in syscalls
		for timeout > 0 {
			if n, err := unix.Poll(s, 50); err != nil {
				break
			} else if n > 0 {
				for i := range s {
					if s[i].Revents&unix.POLLOUT != 0 {
						cbOK(cc[idx[i]])
					}
					if s[i].Revents&unix.POLLERR != 0 || s[i].Revents&unix.POLLHUP != 0 {
						cbErr(cc[idx[i]], errBadConnection)
					}
				}
			}
			timeout -= dur
		}
	})
}
