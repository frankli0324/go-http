package internal

import (
	"crypto/tls"
	"io"
	"net"
)

var defaultDialer = &CoreDialer{
	TLSConfig: &tls.Config{
		NextProtos: []string{"h2"},
	},
}

func getRawConn(c io.ReadWriteCloser) net.Conn {
	if conn, ok := c.(interface{ Raw() net.Conn }); ok {
		return conn.Raw()
	}
	return nil
}

func getTLSConn(c io.ReadWriteCloser) *tls.Conn {
	if tls, ok := getRawConn(c).(*tls.Conn); ok {
		return tls
	}
	return nil
}
