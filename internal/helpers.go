package internal

import (
	"context"

	"github.com/frankli0324/go-http/internal/http"
)

// UseCoreDialer is a shortcut to [UseDialer] that unwraps the current dialer until it meets a [*CoreDialer]
// returns false if no [*CoreDialer] is found
func (c *Client) UseCoreDialer(set func(*CoreDialer) http.Dialer) (ok bool) {
	c.UseDialer(func(d http.Dialer) http.Dialer {
		cd := d
		for cd != nil {
			if d, isCoreD := cd.(*CoreDialer); isCoreD {
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
	c.UseCoreDialer(func(d *CoreDialer) http.Dialer {
		np := d.TLSConfig.NextProtos
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
	return c.UseCoreDialer(func(d *CoreDialer) http.Dialer {
		d.GetProxy = getProxy
		return d
	})
}
