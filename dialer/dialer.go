package dialer

import (
	"github.com/frankli0324/go-http/internal/dialer"
)

// Dialers are responsible for creating underlying streams that http requests could
// be written to and responses could be read from. for example, opening a raw TCP
// connection for HTTP/1.1 requests.
//
// Unlike [net/http.Transport], A Dialer MUST NOT hold active connection states,
// which means a Dialer must be able to be swapped out from a [Client] without
// pain. Like [net/http.Transport], it SHOULD hold the connection related configs
// like [ProxyConfiguration] or *[net/tls.Config].
type Dialer = dialer.Dialer

// CoreDialer is the default implementation of the [Dialer] interface. It would
// be used by a zero value [Client].
type CoreDialer = dialer.CoreDialer

type ProxyConfig = dialer.ProxyConfig

// we need a dedicated resolver for two scenarios:
//
//  1. Resolve remote address locally in proxied requests
//  2. to customize the DNS server used for resolving hostname
//
// the standard library didn't provide a intuitive way of
// setting DNS server addresses since it only follows the
// system configuration (e.g. /etc/resolv.conf), leaving us only
// one option of using [net.Resolver.Dial] hook with a Go Resolver.
//
// this part of code tries to take advantage of that
// only option as far as possible to provide a relativly
// intuitive configuration API.
type ResolveConfig = dialer.ResolveConfig
