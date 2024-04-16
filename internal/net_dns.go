package internal

import (
	"context"
	"net"
)

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

func (d *CoreDialer) LookupIPServer(ctx context.Context, network, host, dns string) ([]net.IP, error) {
	return customServerResolver.LookupIP(dnsServerCtx{ctx, dns}, network, host)
}
