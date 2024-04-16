package internal

import (
	"context"
	"net"
)

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
type ResolveConfig struct {
	CustomDNSServer string
	Network         string            // one of "ip4", "ip6", default is "ip"
	StaticHosts     map[string]string // resembles /etc/hosts
}

func (c *ResolveConfig) Clone() *ResolveConfig {
	if c == nil {
		return nil
	}
	return &ResolveConfig{
		CustomDNSServer: c.CustomDNSServer,
		Network:         c.Network,
		StaticHosts:     c.StaticHosts,
	}
}

// this type should not be used outside this file.
// prevents non-custom DNS server contexts to iterate through all keys
type dnsServerCtx struct {
	context.Context
	server string
}

var dnsServerCtxKey = &dnsServerCtx{nil, "dns-server"} // non-nil pointer to any object, definitely unique

func (c dnsServerCtx) Value(key interface{}) interface{} {
	if key == dnsServerCtxKey {
		return c.server
	}
	return c.Context.Value(key)
}

var customServerResolver = net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		if v, ok := ctx.Value(dnsServerCtxKey).(string); ok && v != "" {
			return zeroDialer.DialContext(ctx, network, v)
		}
		return zeroDialer.DialContext(ctx, network, address)
	},
}

func (d *CoreDialer) lookup(ctx context.Context, cfg *ResolveConfig, host string) (result []net.IP, err error) {
	if cfg == nil {
		return d.LookupIPServer(ctx, "ip", host, "")
	}
	network := cfg.Network
	if network == "" {
		network = "ip"
	}
	return d.LookupIPServer(ctx, network, host, cfg.CustomDNSServer)
}

// LookupIPServer performs DNS lookup for a host on a custom dns server,
// it calls [net.Resolver.LookupIP] with a Go Resolver behind the scenes.
// This part of logic may be reused when wrapping *[CoreDialer] into
// a new custom [Dialer]
func (d *CoreDialer) LookupIPServer(ctx context.Context, network, host, dns string) ([]net.IP, error) {
	return customServerResolver.LookupIP(dnsServerCtx{ctx, dns}, network, host)
}
