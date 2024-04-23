package h2c

import "golang.org/x/net/http2"

// newSettings creates a settings instance with default values
func newSettings(c *Connection) *ClientSettings {
	settings := &ClientSettings{
		HeaderTableSize:        4096,
		EnablePush:             1,
		MaxConcurrentStreams:   1000,
		InitialWindowSize:      65535,
		MaxReadFrameSize:       10 << 20,
		MaxWriteFrameSize:      16384,
		MaxReadHeaderListSize:  10 << 20,
		MaxWriteHeaderListSize: 0xffffffff,
	}
	c.on[http2.FrameSettings] = func(f http2.Frame) {
		sf := f.(*http2.SettingsFrame)
		if sf.IsAck() {
			return
		}
		settings.UpdateFrom(sf)
	}
	return settings
}

const (
	minMaxFrameSize = 1 << 14
	maxMaxFrameSize = 1<<24 - 1
)

// ClientSettings is a set of http2 settings used for *clients*
type ClientSettings struct {
	HeaderTableSize        uint32
	EnablePush             uint32
	MaxConcurrentStreams   uint32
	InitialWindowSize      uint32
	MaxReadFrameSize       uint32
	MaxWriteFrameSize      uint32
	MaxReadHeaderListSize  uint32
	MaxWriteHeaderListSize uint32
	on                     [8][]func(value uint32) // 8 -> max settings id
}

func GetMaxFrameSize(fs uint32) uint32 {
	if fs == 0 {
		return 0 // use the default provided by the peer
	}
	if fs < minMaxFrameSize {
		return minMaxFrameSize
	}
	if fs > maxMaxFrameSize {
		return maxMaxFrameSize
	}
	return fs
}

// On registers callback on server pushed settings to client
func (s *ClientSettings) On(id http2.SettingID, do func(value uint32)) {
	s.on[id] = append(s.on[id], do)
}

func (s *ClientSettings) UpdateFrom(frame *http2.SettingsFrame) {
	frame.ForeachSetting(func(i http2.Setting) error {
		for _, v := range s.on[i.ID] {
			v(i.Val)
		}
		switch i.ID {
		case http2.SettingEnablePush:
			s.EnablePush = i.Val
		case http2.SettingHeaderTableSize:
			s.HeaderTableSize = i.Val
		case http2.SettingInitialWindowSize:
			s.InitialWindowSize = i.Val
		case http2.SettingMaxConcurrentStreams:
			s.MaxConcurrentStreams = i.Val
		case http2.SettingMaxFrameSize:
			s.MaxWriteFrameSize = i.Val
		case http2.SettingMaxHeaderListSize:
			s.MaxWriteHeaderListSize = i.Val
		}
		return nil
	})
}
