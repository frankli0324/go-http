package controller

import (
	"sync"

	"golang.org/x/net/http2"
)

func newSettingsMixin(c *Controller) settingsMixin {
	return settingsMixin{newPeerSettings(c), newSelfSettings(c)}
}

type settingsMixin struct {
	peerSettings, selfSettings *settings
}

func (s settingsMixin) GetPeerSetting(id http2.SettingID) uint32 {
	return s.peerSettings.GetSetting(id)
}
func (s settingsMixin) GetSelfSetting(id http2.SettingID) uint32 {
	return s.selfSettings.GetSetting(id)
}

func (s settingsMixin) ConfigureReadSetting(id http2.SettingID, val uint32) error {
	// shall be called before handshake
	// TODO: if called after handshake, send SETTINGS frame to update
	if err := (http2.Setting{ID: id, Val: val}).Valid(); err != nil {
		return err
	}
	s.selfSettings.settings[id] = val
	for _, v := range s.selfSettings.on[id] {
		v(val)
	}
	return nil
}

func (s settingsMixin) AdvertiseSelfSettings(c *Controller) error {
	settings := make([]http2.Setting, 0, 8)
	for id := 1; id <= 6; id++ {
		setting := http2.Setting{
			ID:  http2.SettingID(id),
			Val: s.selfSettings.settings[id],
		}
		if setting.Valid() == nil {
			settings = append(settings, setting)
		}
	}
	return c.WriteSettings(settings...)
}

func newSelfSettings(_ *Controller) *settings {
	s := [8]uint32{}
	s[http2.SettingHeaderTableSize] = 4096
	s[http2.SettingEnablePush] = 0
	s[http2.SettingMaxConcurrentStreams] = 1000
	s[http2.SettingInitialWindowSize] = 4 << 20
	s[http2.SettingMaxFrameSize] = 10 << 20
	s[http2.SettingMaxHeaderListSize] = 10 << 20 // allow response header to be at most 10MB, whi
	return &settings{settings: s}
}

// newPeerSettings creates a settings instance with default values
func newPeerSettings(c *Controller) *settings {
	s := [8]uint32{}
	s[http2.SettingHeaderTableSize] = 4096
	s[http2.SettingEnablePush] = 1
	s[http2.SettingMaxConcurrentStreams] = 1000
	s[http2.SettingInitialWindowSize] = 65535
	s[http2.SettingMaxFrameSize] = 16384
	s[http2.SettingMaxHeaderListSize] = 0xffffffff
	settings := &settings{settings: s}

	c.on[http2.FrameSettings] = func(f http2.Frame) {
		sf := f.(*http2.SettingsFrame)
		if sf.IsAck() {
			return
		}
		if err := settings.UpdateFrom(sf); err != nil {
			c.GoAwayDebug(0, http2.ErrCodeProtocol, []byte("invalid settings"))
		} else {
			_ = c.WriteSettingsAck()
		}
		// TODO: error acking settings frame
	}
	return settings
}

const (
	minMaxFrameSize = 1 << 14
	maxMaxFrameSize = 1<<24 - 1
)

// settings is a set of http2 settings
type settings struct {
	settings [8]uint32               // http2.SettingID -> Val
	on       [8][]func(value uint32) // 8 -> max settings id
	mu       sync.RWMutex
}

// On registers callback on server pushed settings to client
func (s *settings) On(id http2.SettingID, do func(value uint32)) {
	s.on[id] = append(s.on[id], do)
}

func (s *settings) UpdateFrom(frame *http2.SettingsFrame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return frame.ForeachSetting(func(i http2.Setting) error {
		if err := i.Valid(); err != nil {
			return err
		}
		for _, v := range s.on[i.ID] {
			v(i.Val)
		}
		s.settings[i.ID] = i.Val
		return nil
	})
}

func (s *settings) MaxFrameSize() uint32 {
	fs := s.GetSetting(http2.SettingMaxFrameSize)
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

func (s *settings) MaxHeaderListSize() uint32 {
	st := s.GetSetting(http2.SettingMaxHeaderListSize)
	if st == 0 {
		return 10 << 20
	}
	if st == 0xffffffff {
		return 0
	}
	return st
}

func (s *settings) GetSetting(id http2.SettingID) uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings[id]
}
