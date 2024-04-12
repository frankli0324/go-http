package http

import (
	"net/http"

	"github.com/frankli0324/go-http/internal"
	"github.com/frankli0324/go-http/internal/model"
)

// Client provides the basic API for sending HTTP requests
type Client = internal.Client
type Header = http.Header

// Requests are re-usable objects that are high-level representations
// of a HTTP request. A request would be "prepared" into *[PreparedRequest]s
// to be actually written into an underlying connection (i.e. a TCP stream)
type Request = model.Request
type PreparedRequest = model.PreparedRequest

// Responses are high-level representations of a HTTP response.
type Response = model.Response

// Middleware provides a way to modify *[PreparedRequest]s before actually
// sending it on a remote connection, and perform post-request actions based
// on the received *[Response].
//
// Middlewares are not supposed to be used for modifying *[Client] related configs.
// A common use case is to add trace headers as specified in
// https://w3c.github.io/trace-context/
type Middleware = internal.Middleware
