package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	healthmonitorv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/healthmonitor/v1"
)

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version == "" {
		t.Error("module version must not be empty")
	}
	if info.HTTPAddr != ":9203" {
		t.Errorf("HTTPAddr = %q, want :9203", info.HTTPAddr)
	}
}

func TestModuleLifecycle(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0", Interval: time.Hour})
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

func TestReportHealthDegradedTransition(t *testing.T) {
	m := NewModule(Config{Interval: time.Hour})
	ctx := context.Background()

	_, err := m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "api-rest", State: "running"},
		},
	})
	if err != nil {
		t.Fatalf("ReportHealth running: %v", err)
	}
	st := m.SnapshotForTest()
	if st.Status != "ok" || st.ModuleCount != 1 {
		t.Fatalf("after running: %+v", st)
	}

	_, err = m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "api-rest", State: "degraded", Error: "timeout"},
		},
	})
	if err != nil {
		t.Fatalf("ReportHealth degraded: %v", err)
	}
	st = m.SnapshotForTest()
	if st.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", st.Status)
	}
	if st.DegradedCount != 1 {
		t.Fatalf("degraded_transitions = %d, want 1", st.DegradedCount)
	}
	if len(st.RecentEvents) == 0 || st.RecentEvents[0].EventType != "module.degraded" {
		t.Fatalf("expected module.degraded event, got %+v", st.RecentEvents)
	}
}

func TestPublishEventStored(t *testing.T) {
	m := NewModule(Config{Interval: time.Hour})
	_, err := m.PublishEvent(context.Background(), &healthmonitorv1.PublishEventRequest{
		EventType: "custom.alert",
		ModuleId:  "media-movies",
		Message:   "disk low",
	})
	if err != nil {
		t.Fatalf("PublishEvent: %v", err)
	}
	st := m.SnapshotForTest()
	if st.EventsPublish != 1 {
		t.Fatalf("events_published = %d", st.EventsPublish)
	}
	if len(st.RecentEvents) != 1 || st.RecentEvents[0].Message != "disk low" {
		t.Fatalf("recent_events = %+v", st.RecentEvents)
	}
}

func TestMeshFanoutOnDegraded(t *testing.T) {
	m := NewModule(Config{Interval: time.Hour})
	var gotType, gotSrc string
	m.meshPub = func(ctx context.Context, eventType, source string, payload []byte) error {
		gotType, gotSrc = eventType, source
		return nil
	}
	ctx := context.Background()
	_, _ = m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{{ModuleId: "api-rest", State: "running"}},
	})
	_, err := m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "api-rest", State: "degraded", Error: "timeout"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotType != "module.degraded" || gotSrc != "health-monitor" {
		t.Fatalf("mesh publish type=%q src=%q", gotType, gotSrc)
	}
	if m.SnapshotForTest().MeshPublished != 1 {
		t.Fatalf("mesh_published = %d", m.SnapshotForTest().MeshPublished)
	}
}

func TestStaleExpiry(t *testing.T) {
	m := NewModule(Config{Interval: 100 * time.Millisecond})
	ctx := context.Background()
	_, err := m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "old-mod", State: "running"},
		},
	})
	if err != nil {
		t.Fatalf("ReportHealth: %v", err)
	}
	m.SetLastSeenForTest("old-mod", time.Now().UTC().Add(-time.Second))
	m.MarkStaleForTest()
	st := m.SnapshotForTest()
	if st.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", st.Status)
	}
	if st.StaleCount != 1 {
		t.Fatalf("stale_count = %d, want 1", st.StaleCount)
	}
	found := false
	for _, mod := range st.Modules {
		if mod.ModuleID == "old-mod" && (mod.Stale || mod.State == "stale") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected stale module, got %+v", st.Modules)
	}
}

func TestStatusHTTP(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0", Interval: time.Hour})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop(ctx) }()

	_, _ = m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "core", State: "running"},
		},
	})

	addr := m.httpLis.Addr().String()
	resp, err := http.Get("http://" + addr + "/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("status code %d", resp.StatusCode)
	}
	var st statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.ModuleCount != 1 || st.Status != "ok" {
		t.Fatalf("unexpected status: %+v", st)
	}
}
