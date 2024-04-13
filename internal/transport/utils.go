package transport

import "io"

type Releaser interface {
	Release()
}

type bodyCloser struct {
	io.Reader
	close func() error
}

func (b bodyCloser) Close() error {
	return b.close()
}
