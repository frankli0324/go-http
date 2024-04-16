package internal_test

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/frankli0324/go-http/internal"
	"github.com/frankli0324/go-http/internal/model"
)

func TestClientHTTP2(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()

	client := &internal.Client{}
	client.UseCoreDialer(func(cd *internal.CoreDialer) internal.Dialer {
		cd.TLSConfig.RootCAs, _ = x509.SystemCertPool()
		cd.TLSConfig.RootCAs.AddCert(server.Certificate())
		return cd
	})
	fmt.Println(client.CtxDo(context.Background(), &model.Request{
		Method: "GET", URL: server.URL,
	}))
}
