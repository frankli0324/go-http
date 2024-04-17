package http

import (
	"github.com/frankli0324/go-http/internal"
	"github.com/frankli0324/go-http/internal/http"
)

// Client provides the basic API for sending HTTP requests
type Client = internal.Client
type Header = http.Header

// Requests are re-usable objects that are high-level representations
// of a HTTP request. A request would be "prepared" into *[PreparedRequest]s
// to be actually written into an underlying connection (i.e. a TCP stream)
type Request = http.Request

// Responses are high-level representations of a HTTP response.
type Response = http.Response
