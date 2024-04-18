//go:build darwin || linux
// +build darwin linux

package nettools

import (
	"math/rand"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

var _ = func() error { // make sure this executes before func init()
	supported[ModePoll] = pollForWrite
	return nil
}()

func pollForWrite(cc []net.Conn, timeout time.Duration) (canWrite net.Conn) {
	rconns, remaining := getAllSysRawConns(cc)
	controlFDSet(rconns, func(u []int) {
		s := make([]unix.PollFd, 0, len(u))
		idx := make([]int, 0, len(u))
		for i, v := range u {
			if v == -1 {
				continue
			}
			s = append(s, unix.PollFd{Fd: int32(v), Events: unix.POLLOUT})
			idx = append(idx, i)
		}

		dur := time.Millisecond * 50 // process shouldn't hang in syscalls
		for timeout > 0 {
			if n, err := unix.Poll(s, 50); err != nil {
				break
			} else if n > 0 {
				for i := range s {
					if s[i].Revents&unix.POLLOUT != 0 {
						canWrite = cc[idx[i]]
					}
				}
			}
			timeout -= dur
		}
	})
	if canWrite == nil && len(remaining) > 0 {
		return remaining[rand.Intn(len(remaining))]
	}
	return
}
