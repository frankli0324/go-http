package nettools

import (
	"math/rand"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

var _ = func() error { // make sure this executes before func init()
	supported[ModeSelect] = selectForWrite
	return nil
}()

func selectForWrite(cc []net.Conn, timeout time.Duration) (canWrite net.Conn) {
	rconns, remaining := getAllSysRawConns(cc)
	controlFDSet(rconns, func(u []int) {
		tv := unix.NsecToTimeval(int64(50 * time.Millisecond))
		idx := make([]int, 0, len(u))
		var set unix.FdSet
		for i, fd := range u {
			if fd == -1 {
				continue
			}
			set.Set(int(fd))
			idx = append(idx, i)
		}
		nfds := max(u)

		for timeout > 0 {
			if n, err := unix.Select(int(nfds), nil, &set, nil, &tv); err != nil {
				break
			} else if n > 0 {
				for i, fd := range u {
					if set.IsSet(int(fd)) {
						canWrite = cc[idx[i]]
						return
					}
				}
			}
			timeout -= 50 * time.Millisecond
		}
	})
	if canWrite == nil && len(remaining) > 0 {
		return remaining[rand.Intn(len(remaining))]
	}
	return
}

func max(nums []int) int {
	if len(nums) == 0 {
		return 0
	}
	m := nums[0]
	for i := 1; i < len(nums); i++ {
		if nums[i] > m {
			m = nums[i]
		}
	}
	return m
}
