// package http contains the request and response type, which are meant
// to be exported. the package name is meant to be same with the top
// level package name so that IDEs and code editors could pick them up
//
// the package also contains some type and value aliases from standard
// library to avoid annoying imports
package http

import (
	"net/http"
)

type Header = http.Header

var NoBody = http.NoBody
