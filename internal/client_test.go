package internal_test

import (
	"net/http"
	"testing"
	"testing/iotest"

	"github.com/frankli0324/go-http/internal/model"
)

type tCase struct {
	data []byte
	req  *model.Request
}

var reqShouldBe = map[string]tCase{
	"BasicRequest": {
		req: &model.Request{
			Method: "GET",
			URL:    "http://www.example.com",
		},
		data: []byte("GET / HTTP/1.1\r\nHost: www.example.com\r\n\r\n"),
	},
	"QueryNonStandard": {
		req: &model.Request{
			Method: "GET",
			URL:    "http://www.example.com/test?1=33=1",
		},
		data: []byte("GET /test?1=33=1 HTTP/1.1\r\nHost: www.example.com\r\n\r\n"),
	},
	"HeaderNotCanonicalized": {
		req: &model.Request{
			Method: "GET",
			URL:    "http://www.example.com/",
			Header: http.Header{"x-123-vv": {"1"}},
		},
		data: []byte("GET / HTTP/1.1\r\nHost: www.example.com\r\nx-123-vv: 1\r\n\r\n"),
	},
	"URIFragmentNotIncluded": {
		req: &model.Request{
			Method: "GET",
			URL:    "http://www.example.com/?test=1#frag",
		},
		data: []byte("GET /?test=1 HTTP/1.1\r\nHost: www.example.com\r\n\r\n"),
	},
}

func TestRequestSerialize(t *testing.T) {
	for name, cas := range reqShouldBe {
		tCase := cas
		t.Run(name, func(t *testing.T) {
			req := SendSingleRequest(t, tCase.req)
			if err := iotest.TestReader(req, tCase.data); err != nil {
				t.Error(err)
			}
		})
	}
}
