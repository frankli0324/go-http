package internal

import (
	"context"

	"github.com/frankli0324/go-http/dialer"
	"github.com/frankli0324/go-http/internal/http"
)

// UseCoreDialer is a shortcut to [UseDialer] that unwraps the current dialer until it meets a [*CoreDialer]
// returns false if no [*CoreDialer] is found
func (c *Client) UseCoreDialer(set func(*dialer.CoreDialer) dialer.Dialer) (ok bool) {
	c.UseDialer(func(d dialer.Dialer) dialer.Dialer {
		cd := d
		for cd != nil {
			if d, isCoreD := cd.(*dialer.CoreDialer); isCoreD {
				ok = true
				return set(d)
			}
			cd = cd.Unwrap()
		}
		return d
	})
	return
}

func (c *Client) DisableH2() (ok bool) {
	c.UseCoreDialer(func(d *dialer.CoreDialer) dialer.Dialer {
		np := d.TLSConfig.NextProtos
		if len(np) == 1 && np[0] == "h2" {
			d.TLSConfig.NextProtos = nil
			ok = true
			return d
		}
		for i := range np {
			if np[i] == "h2" {
				d.TLSConfig.NextProtos = append(np[:i], np[i+1:]...)
				ok = true
				return d
			}
		}
		return d
	})
	return
}

func (c *Client) UseGetProxy(getProxy func(ctx context.Context, r *http.Request) (string, error)) (ok bool) {
	return c.UseCoreDialer(func(d *dialer.CoreDialer) dialer.Dialer {
		d.GetProxy = getProxy
		return d
	})
}
