package controller

import (
	"sync"

	"golang.org/x/net/http2"
)

func newSettingsMixin() settingsMixin {
	return settingsMixin{
		peerSettings: newPeerSettings(),
		selfSettings: newSelfSettings(),
	}
}

type settingsMixin struct {
	muPeer, muSelf             sync.RWMutex
	peerSettings, selfSettings settings
}

func (s *settingsMixin) UsePeerSetting(id http2.SettingID) (uint32, func()) {
	s.muPeer.RLock()
	return s.peerSettings.v[id], s.muPeer.RUnlock
}

func (s *settingsMixin) UseSelfSetting(id http2.SettingID) (uint32, func()) {
	s.muSelf.RLock()
	return s.selfSettings.v[id], s.muSelf.RUnlock
}

func (s *settingsMixin) UpdatePeerSettings(sf *http2.SettingsFrame) (func(), error) {
	s.muPeer.Lock()
	return s.muPeer.Unlock, s.peerSettings.UpdateFrom(sf)
}

func (s *settingsMixin) ConfigureReadSetting(id http2.SettingID, val uint32) error {
	// shall be called before handshake
	if err := (http2.Setting{ID: id, Val: val}).Valid(); err != nil {
		return err
	}
	s.selfSettings.v[id] = val
	return nil
}

func (s *settingsMixin) advertiseSettings(c *Controller) error {
	settings := make([]http2.Setting, 0, 8)
	for id := 1; id <= 6; id++ {
		setting := http2.Setting{
			ID:  http2.SettingID(id),
			Val: s.selfSettings.v[id],
		}
		if setting.Valid() == nil {
			settings = append(settings, setting)
		}
	}
	return c.WriteSettings(settings...)
}

func newSelfSettings() settings {
	s := [8]uint32{}
	s[http2.SettingHeaderTableSize] = 4096
	s[http2.SettingEnablePush] = 0
	s[http2.SettingMaxConcurrentStreams] = 1000
	s[http2.SettingInitialWindowSize] = 4 << 20

	// allow response header to be at most 10MB, whi
	s[http2.SettingMaxHeaderListSize] = 10 << 20

	// note that [golang.org/x/net/http2.Framer] only
	// supports read frame size up to 1<<24 - 1
	s[http2.SettingMaxFrameSize] = 16 << 20

	return settings{v: s}
}

// newPeerSettings creates a settings instance with default values
func newPeerSettings() settings {
	s := [8]uint32{}
	s[http2.SettingHeaderTableSize] = 4096
	s[http2.SettingEnablePush] = 1
	s[http2.SettingMaxConcurrentStreams] = 1000
	s[http2.SettingInitialWindowSize] = 65535
	s[http2.SettingMaxFrameSize] = 16384
	s[http2.SettingMaxHeaderListSize] = 0xffffffff
	settings := settings{v: s}
	return settings
}

// settings is a set of http2 settings
type settings struct {
	v [8]uint32 // http2.SettingID -> Val
}

func (s *settings) UpdateFrom(frame *http2.SettingsFrame) error {
	return frame.ForeachSetting(func(i http2.Setting) error {
		if err := i.Valid(); err != nil {
			return err
		}
		if i.ID == http2.SettingEnablePush && i.Val == 1 {
			// A *client** MUST treat receipt of a SETTINGS frame with SETTINGS_ENABLE_PUSH
			// set to 1 as a connection error (Section 5.4.1) of type PROTOCOL_ERROR.
			return http2.ConnectionError(http2.ErrCodeProtocol)
		}
		if i.ID == 0 || i.ID > 6 {
			// An endpoint that receives a SETTINGS frame with any
			// unknown or unsupported identifier MUST ignore that setting.
			return nil
		}
		s.v[i.ID] = i.Val
		return nil
	})
}
