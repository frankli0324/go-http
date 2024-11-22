// package transport contains implementations to requirements on *message syntaxes*
// defined by http related RFCs.
//
// as of 2022.06, RFCs that were to define HTTP/1.1 (RFC753x) are obsoleted by:
//
//  HTTP Semantics (RFC9110)
//  HTTP Caching (RFC9111) and
//  HTTP/1.1 (RFC9112)
//
// and HTTP/2.0 and HTTP/3.0 is also going to be defined in RFC9113 and RFC9114 respectively, while
// they're still under proposed state.
//
// net/http components are reused on the "semantics" part ([net/http.URL], [net/http.Header], etc.)

package transport
