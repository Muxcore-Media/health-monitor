package internal

import (
	"testing"
	"time"
)

func TestSettingsPollInterval(t *testing.T) {
	m := NewModule(Config{Interval: 30 * time.Second})
	defs := m.Settings()
	if len(defs) != 1 || defs[0].Key != "poll_interval" {
		t.Fatalf("unexpected defs: %+v", defs)
	}
	if defs[0].Value != "30s" {
		t.Fatalf("value=%q", defs[0].Value)
	}
	if err := m.UpdateSetting("poll_interval", "15s"); err != nil {
		t.Fatal(err)
	}
	if got := m.getInterval(); got != 15*time.Second {
		t.Fatalf("interval=%v", got)
	}
	if err := m.UpdateSetting("HEALTH_MONITOR_INTERVAL", "45s"); err != nil {
		t.Fatal(err)
	}
	if got := m.getInterval(); got != 45*time.Second {
		t.Fatalf("interval=%v", got)
	}
	if err := m.UpdateSetting("poll_interval", "nope"); err == nil {
		t.Fatal("expected invalid duration error")
	}
	if err := m.UpdateSetting("unknown", "1s"); err == nil {
		t.Fatal("expected unknown setting error")
	}
}
