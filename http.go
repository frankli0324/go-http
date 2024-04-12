package http

import (
	"net/http"

	"github.com/frankli0324/go-http/internal"
	"github.com/frankli0324/go-http/internal/model"
)

type Client = internal.Client
type Header = http.Header
type Request = model.Request
type PreparedRequest = model.PreparedRequest
type Response = model.Response

type Middleware = internal.Middleware
