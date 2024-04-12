package internal

func (c *Client) DisableH2() (ok bool) {
	c.UseDialer(func(d Dialer) Dialer {
		cd := d
		for cd != nil {
			if d, ok := cd.(*CoreDialer); ok {
				np := d.TLSConfig.NextProtos
				for i := range np {
					if np[i] == "h2" {
						d.TLSConfig.NextProtos = append(np[:i], np[i+1:]...)
						ok = true
						break
					}
				}
			}
			cd = cd.Unwrap()
		}
		return d
	})
	return
}
