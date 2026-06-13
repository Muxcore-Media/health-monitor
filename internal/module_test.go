package internal

import (
	"context"
	"testing"

	healthmonitorv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/healthmonitor/v1"
)

func testConfig() Config {
	return Config{GRPCAddr: ":0", HTTPAddr: ":0"}
}

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version == "" {
		t.Error("module version must not be empty")
	}
	if info.MinCoreVersion == "" {
		t.Error("MinCoreVersion must not be empty")
	}
	if len(info.Contracts) == 0 {
		t.Error("Contracts must not be empty")
	}
}

func TestModuleLifecycle(t *testing.T) {
	m := NewModule(testConfig())
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestReportHealth(t *testing.T) {
	m := NewModule(testConfig())
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer m.Stop(ctx)

	resp, err := m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "mod-b", State: "running"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %s", resp.Status)
	}

	resp, err = m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "mod-b", State: "degraded", Error: "high latency"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %s", resp.Status)
	}
	if m.degradedCount.Load() != 1 {
		t.Fatalf("expected 1 degraded count, got %d", m.degradedCount.Load())
	}
}

func TestPublishEvent(t *testing.T) {
	m := NewModule(testConfig())
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer m.Stop(ctx)

	resp, err := m.PublishEvent(ctx, &healthmonitorv1.PublishEventRequest{
		EventType: "module.crash", ModuleId: "mod-a", Message: "OOM",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %s", resp.Status)
	}
	if m.eventsPublish.Load() != 1 {
		t.Fatalf("expected 1 event, got %d", m.eventsPublish.Load())
	}
}
