package internal

import (
	"fmt"
	"strings"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func (m *Module) Settings() []contracts.SettingDef {
	return m.settingsDefs()
}

func (m *Module) UpdateSetting(key, value string) error {
	return m.updateSetting(key, value)
}

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.mu.Lock()
	interval := m.interval
	m.mu.Unlock()
	return []contracts.SettingDef{
		{
			Key:         "poll_interval",
			Label:       "Stale Poll Interval",
			Type:        contracts.SettingTypeString,
			Value:       interval.String(),
			Description: "How often to mark modules stale (Go duration, e.g. 30s). Env: HEALTH_MONITOR_INTERVAL",
			Group:       "Monitoring",
			Required:    false,
		},
	}
}

func (m *Module) updateSetting(key, value string) error {
	switch key {
	case "poll_interval", "HEALTH_MONITOR_INTERVAL":
		d, err := time.ParseDuration(strings.TrimSpace(value))
		if err != nil || d <= 0 {
			return fmt.Errorf("invalid poll_interval %q (use Go duration like 30s)", value)
		}
		m.mu.Lock()
		m.interval = d
		m.mu.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

func (m *Module) getInterval() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.interval
}
