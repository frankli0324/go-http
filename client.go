package http

import (
	"net/http"

	"github.com/frankli0324/go-http/internal"
	"github.com/frankli0324/go-http/internal/model"
)

type Header = http.Header
type Client = internal.Client
type Request = model.Request
type Middleware = model.Middleware
