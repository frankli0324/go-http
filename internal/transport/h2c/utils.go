package h2c

import "sync"

func min(x int, y ...int) int {
	for _, y := range y {
		if y < x {
			x = y
		}
	}
	return x
}

type bufPool struct {
	pools  []sync.Pool
	limits []int
}

func (bp *bufPool) init(sizes, limits []int) *bufPool {
	if len(sizes) != len(limits) {
		panic("buf pool unequal sizes")
	}
	bp.pools = make([]sync.Pool, len(sizes))
	bp.limits = limits
	for i := 0; i < len(sizes); i++ {
		sz := i
		bp.pools[i] = sync.Pool{New: func() interface{} {
			b := make([]byte, sz)
			return &b
		}}
	}
	return bp
}

func (bp *bufPool) get(sz int) (*[]byte, int) {
	for i := 0; i < len(bp.limits); i++ {
		if sz <= bp.limits[i] {
			b := bp.pools[i].Get().(*[]byte)
			if sz > len(*b) {
				*b = append(*b, make([]byte, sz-len(*b))...)
			}
			return b, i
		}
	}
	return nil, -1
}

func (bp *bufPool) put(idx int, buf *[]byte) {
	bp.pools[idx].Put(buf)
}
