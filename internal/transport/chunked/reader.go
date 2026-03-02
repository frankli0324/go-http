package chunked

import (
	"bufio"
	"errors"
	"io"
)

func NewChunkedReader(r io.Reader) io.Reader {
	var br *bufio.Reader
	if v, ok := r.(*bufio.Reader); ok {
		br = v
	} else {
		br = bufio.NewReader(r)
	}
	return &chunkedReader{br, 0, -1}
}

type chunkedReader struct {
	*bufio.Reader
	currentCount, currentChunkSize int64
}

func (c *chunkedReader) readChunkHeader() (len uint64, err error) {
	cnt := 0
	isPref := true
	for isPref {
		var line []byte
		line, isPref, err = c.ReadLine()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return 0, err
		}
		for _, b := range line {
			cnt++
			switch {
			case '0' <= b && b <= '9':
				b = b - '0'
			case 'a' <= b && b <= 'f':
				b = b - 'a' + 10
			case 'A' <= b && b <= 'F':
				b = b - 'A' + 10
			default:
				return 0, errors.New("invalid byte in chunk length")
			}
			len <<= 4
			len |= uint64(b)
		}
		if cnt >= 16 {
			return 0, errors.New("http chunk length too large")
		}
	}
	return
}

func (c *chunkedReader) Read(p []byte) (n int, err error) {
	if c.currentChunkSize == -1 {
		l, err := c.readChunkHeader()
		if err != nil {
			return n, err
		}
		c.currentChunkSize = int64(l)
	}
	if c.currentChunkSize != 0 {
		if int(c.currentChunkSize-c.currentCount) < len(p) {
			p = p[:c.currentChunkSize-c.currentCount]
		}
		n, err = c.Reader.Read(p)
		c.currentCount += int64(n)
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return n, err
		}
	}
	if c.currentCount == c.currentChunkSize {
		err = nil
		dr, _ := c.Reader.ReadByte()
		dn, err := c.Reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return n, err
		}
		if dr != '\r' || dn != '\n' {
			return n, errors.New("malformed chunked encoding")
		}
		if c.currentChunkSize == 0 {
			return n, io.EOF
		}
		c.currentCount = 0
		c.currentChunkSize = -1
	}
	return
}
