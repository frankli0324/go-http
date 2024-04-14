package h2c

import "golang.org/x/net/http2/hpack"

// encode HEADERS frame BlockFragment for given request
func (c *Connection) EncodeHeaders(enumHeaders func(func(k, v string))) ([]byte, error) {
	enumHeaders(func(name, value string) {
		f := hpack.HeaderField{Name: name, Value: value}
		// total += f.Size()
		// if total > settings.max header size { error }
		c.he.WriteField(f)
	})
	return c.hbuf.Bytes(), nil
}
