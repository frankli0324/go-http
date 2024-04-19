package nettools

import (
	"net"
	"time"

	"golang.org/x/sys/unix"
)

var _ = func() error { // make sure this executes before func init()
	supported[ModeSelect] = selectForWrite
	return nil
}()

func selectForWrite(cc []net.Conn, cbUnsure, cbOK func(net.Conn), cbErr func(net.Conn, error), timeout time.Duration) {
	controlFDSet(cc, func(u []int) {
		tv := unix.NsecToTimeval(int64(50 * time.Millisecond))
		idx := make([]int, 0, len(u))
		var set unix.FdSet
		for i, fd := range u {
			if fd == -1 || fd > 1024 {
				cbUnsure(cc[i])
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
						cbOK(cc[i])
					} else { // check for errors
						cbUnsure(cc[idx[i]])
					}
				}
			}
			timeout -= 50 * time.Millisecond
		}
	})
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
