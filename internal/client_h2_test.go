package internal_test

import (
	"context"
	"crypto/x509"
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/frankli0324/go-http/internal"
	"github.com/frankli0324/go-http/internal/dialer"
	"github.com/frankli0324/go-http/internal/http"
)

func TestClientHTTP2(t *testing.T) {
	server := httptest.NewUnstartedServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(200)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()

	client := &internal.Client{}
	client.UseCoreDialer(func(cd *dialer.CoreDialer) dialer.Dialer {
		cd.TLSConfig.RootCAs, _ = x509.SystemCertPool()
		cd.TLSConfig.RootCAs.AddCert(server.Certificate())
		return cd
	})
	fmt.Println(client.CtxDo(context.Background(), &http.Request{
		Method: "GET", URL: server.URL,
	}))
}
