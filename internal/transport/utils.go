package transport

import (
	"io"
	"strings"
)

type bodyCloser struct {
	io.Reader
	close func() error
}

func (b bodyCloser) Close() error {
	return b.close()
}

func Cut(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

func tryRelease(r interface{}) bool {
	rr, ok := r.(interface{ Release() })
	if ok {
		rr.Release()
	}
	return ok
}
