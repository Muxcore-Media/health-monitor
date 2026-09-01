package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
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
	if info.HTTPAddr != defaultHTTPAddr {
		t.Errorf("HTTPAddr = %q, want %q", info.HTTPAddr, defaultHTTPAddr)
	}
	if len(info.Contracts) != 1 || info.Contracts[0].Interface != "HealthMonitor" {
		t.Fatalf("contracts = %+v", info.Contracts)
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

func TestHealthRequiresMeshWhenConfigured(t *testing.T) {
	t.Setenv("MUXCORE_GRPC_ADDR", "127.0.0.1:1")
	m := NewModule(Config{Interval: time.Hour})
	if err := m.Health(context.Background()); err == nil {
		t.Fatal("expected error when mesh configured but not connected")
	}
	m.SetMeshConnectedForTest(true)
	if err := m.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
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
	if len(st.RecentEvents) == 0 || st.RecentEvents[0].EventType != contracts.EventModuleDegraded {
		t.Fatalf("expected module.degraded event, got %+v", st.RecentEvents)
	}
}

func TestReportHealthFirstSeenDegraded(t *testing.T) {
	m := NewModule(Config{Interval: time.Hour})
	ctx := context.Background()
	_, err := m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "new-mod", State: "degraded", Error: "boot failed"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	st := m.SnapshotForTest()
	if st.DegradedCount != 1 {
		t.Fatalf("degraded_transitions = %d", st.DegradedCount)
	}
	if len(st.RecentEvents) != 1 || st.RecentEvents[0].EventType != contracts.EventModuleDegraded {
		t.Fatalf("recent_events = %+v", st.RecentEvents)
	}
}

func TestReportHealthRecovered(t *testing.T) {
	m := NewModule(Config{Interval: time.Hour})
	ctx := context.Background()
	_, _ = m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "api-rest", State: "degraded", Error: "timeout"},
		},
	})
	_, err := m.ReportHealth(ctx, &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "api-rest", State: "running"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	st := m.SnapshotForTest()
	if st.Status != "ok" {
		t.Fatalf("status = %q, want ok", st.Status)
	}
	found := false
	for _, ev := range st.RecentEvents {
		if ev.EventType == contracts.EventModuleRecovered {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected recovered event, got %+v", st.RecentEvents)
	}
}

func TestReportHealthSkipsEmptyModuleID(t *testing.T) {
	m := NewModule(Config{Interval: time.Hour})
	_, err := m.ReportHealth(context.Background(), &healthmonitorv1.ReportHealthRequest{
		Modules: []*healthmonitorv1.ModuleHealth{
			{ModuleId: "", State: "running"},
			{ModuleId: "core", State: "running"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.SnapshotForTest().ModuleCount != 1 {
		t.Fatalf("expected one module tracked")
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
	if len(st.RecentEvents) != 1 || st.RecentEvents[0].Error != "disk low" {
		t.Fatalf("recent_events = %+v", st.RecentEvents)
	}
}

func TestMeshFanoutOnDegraded(t *testing.T) {
	m := NewModule(Config{Interval: time.Hour})
	var gotType, gotSrc string
	var gotPayload contracts.ModuleDegradedPayload
	m.SetMeshPublisherForTest(func(ctx context.Context, eventType, source string, payload []byte) error {
		gotType, gotSrc = eventType, source
		_ = json.Unmarshal(payload, &gotPayload)
		return nil
	})
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
	if gotType != contracts.EventModuleDegraded || gotSrc != "health-monitor" {
		t.Fatalf("mesh publish type=%q src=%q", gotType, gotSrc)
	}
	if gotPayload.ModuleID != "api-rest" || gotPayload.Error != "timeout" {
		t.Fatalf("payload = %+v", gotPayload)
	}
	if m.SnapshotForTest().MeshPublished != 1 {
		t.Fatalf("mesh_published = %d", m.SnapshotForTest().MeshPublished)
	}
}

func TestCustomAlertStoredNotPublished(t *testing.T) {
	m := NewModule(Config{Interval: time.Hour})
	published := false
	m.SetMeshPublisherForTest(func(ctx context.Context, eventType, source string, payload []byte) error {
		published = true
		return nil
	})
	_, err := m.PublishEvent(context.Background(), &healthmonitorv1.PublishEventRequest{
		EventType: "custom.alert",
		ModuleId:  "media-movies",
		Message:   "disk low",
	})
	if err != nil {
		t.Fatal(err)
	}
	if published {
		t.Fatal("custom.alert must not be published to mesh")
	}
	if len(m.SnapshotForTest().RecentEvents) != 1 {
		t.Fatal("expected event stored locally")
	}
}

func TestRecentEventsRingTrim(t *testing.T) {
	m := NewModule(Config{Interval: time.Hour})
	ctx := context.Background()
	for i := 0; i < maxRecentEvents+5; i++ {
		_, err := m.PublishEvent(ctx, &healthmonitorv1.PublishEventRequest{
			EventType: "health.note",
			ModuleId:  "core",
			Message:   "note",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	st := m.SnapshotForTest()
	if len(st.RecentEvents) != maxRecentEvents {
		t.Fatalf("recent_events len = %d, want %d", len(st.RecentEvents), maxRecentEvents)
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

func TestHealthHTTP(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0", Interval: time.Hour})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop(ctx) }()

	addr := m.httpLis.Addr().String()
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("status code %d", resp.StatusCode)
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

func TestProbeModuleHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	host := srv.Listener.Addr().String()
	m := NewModule(Config{Interval: time.Hour})
	state, errMsg := m.probeModuleHealth(context.Background(), "demo", host)
	if state != "running" || errMsg != "" {
		t.Fatalf("state=%q err=%q", state, errMsg)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	state, errMsg = m.probeModuleHealth(context.Background(), "demo", bad.Listener.Addr().String())
	if state != "degraded" || errMsg == "" {
		t.Fatalf("state=%q err=%q", state, errMsg)
	}
}

func TestHealthURLForModule(t *testing.T) {
	t.Setenv("MUXCORE_MESH_DIAL_LOCAL", "true")
	got := healthURLForModule("admin-ui", ":8082")
	want := "http://127.0.0.1:8082/health"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestMeshEventAllowed(t *testing.T) {
	if !meshEventAllowed("module.degraded") {
		t.Fatal("module.degraded should be allowed")
	}
	if meshEventAllowed("custom.alert") {
		t.Fatal("custom.alert should not be allowed")
	}
}

func TestMain(m *testing.M) {
	_ = os.Unsetenv("MUXCORE_GRPC_ADDR")
	os.Exit(m.Run())
}
