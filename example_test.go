package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

func ExampleClient() {
	cl := &Client{}
	resp, err := cl.CtxDo(context.Background(), &Request{
		Method: "GET",
		URL:    "http://www.google.com/?a=b",
		Header: http.Header{
			// "Connection": {"close"},
		},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	fmt.Println(err)
	fmt.Println(string(b))
}
