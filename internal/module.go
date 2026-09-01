package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	healthmonitorv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/healthmonitor/v1"
	"github.com/Muxcore-Media/core/sdk/go/client"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
)

const (
	maxRecentEvents = 50
	defaultGRPCAddr = "127.0.0.1:9202"
	defaultHTTPAddr = "127.0.0.1:9203"
	moduleVersion   = "0.1.6"
)

type trackedHealth struct {
	ModuleID string
	State    string
	Error    string
	LastSeen time.Time
	Stale    bool
}

type healthEvent struct {
	EventType string    `json:"event_type"`
	ModuleID  string    `json:"module_id"`
	Error     string    `json:"error,omitempty"`
	At        time.Time `json:"at"`
}

// meshPublisher fans health events onto the core Events bus (tests may stub).
type meshPublisher func(ctx context.Context, eventType, source string, payload []byte) error

type Module struct {
	healthmonitorv1.UnimplementedHealthMonitorServiceServer
	mu            sync.Mutex
	moduleHealth  map[string]*trackedHealth
	recentEvents  []healthEvent
	eventsPublish atomic.Int64
	meshPublishN  atomic.Int64
	degradedCount atomic.Int64
	staleCount    atomic.Int64
	meshConnected atomic.Bool

	mc         *client.Client
	meshPub    meshPublisher
	httpClient *http.Client
	grpcSrv    *grpc.Server
	grpcLis    net.Listener
	httpLis    net.Listener
	httpSrv    *http.Server
	cancel     context.CancelFunc
	id         string
	grpcAddr   string
	httpAddr   string
	interval   time.Duration
}

type Config struct {
	ID       string
	GRPCAddr string
	HTTPAddr string
	Interval time.Duration
}

// DefaultHTTPAddr returns the default loopback HTTP listen address.
func DefaultHTTPAddr() string {
	return defaultHTTPAddr
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "health-monitor"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = defaultGRPCAddr
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if v := os.Getenv("HEALTH_MONITOR_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("HEALTH_MONITOR_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := os.Getenv("HEALTH_MONITOR_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Interval = d
		}
	}
	return &Module{
		id:           cfg.ID,
		grpcAddr:     cfg.GRPCAddr,
		httpAddr:     cfg.HTTPAddr,
		interval:     cfg.Interval,
		moduleHealth: make(map[string]*trackedHealth),
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Health Monitor",
		Version:      moduleVersion,
		Roles:        []string{"infrastructure"},
		Description:  "Aggregated module health monitoring and degradation detection",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityHealthMonitor, "settings"},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/core/pkg/contracts",
				Interface: "HealthMonitor",
				Version:   "v0.5.8",
			},
		},
		MinCoreVersion: "0.5.8",
		HTTPAddr:       m.httpAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	var err error
	m.grpcLis, err = net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.httpLis, err = net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen HTTP %s: %w", m.httpAddr, err)
	}
	slog.Info("health-monitor initialized", "grpc", m.grpcAddr, "http", m.httpAddr, "interval", m.getInterval())
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	loopCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.staleLoop(loopCtx)
	go m.dialCoreLoop(loopCtx)

	m.grpcSrv = grpc.NewServer()
	healthmonitorv1.RegisterHealthMonitorServiceServer(m.grpcSrv, m)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)
	go func() {
		slog.Info("health-monitor gRPC started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.grpcLis); err != nil {
			slog.Error("health-monitor gRPC error", "error", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", m.handleHealth)
	mux.HandleFunc("/status", m.handleStatus)
	m.httpSrv = &http.Server{Handler: mux}
	go func() {
		slog.Info("health-monitor HTTP started", "addr", m.httpAddr)
		if err := m.httpSrv.Serve(m.httpLis); err != nil && err != http.ErrServerClosed {
			slog.Error("health-monitor HTTP error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.httpSrv != nil {
		_ = m.httpSrv.Shutdown(ctx)
	}
	m.mu.Lock()
	if m.mc != nil {
		_ = m.mc.Close()
		m.mc = nil
	}
	m.meshConnected.Store(false)
	m.mu.Unlock()
	slog.Info("health-monitor stopped")
	return nil
}

func (m *Module) dialCoreLoop(ctx context.Context) {
	meshAddr := strings.TrimSpace(os.Getenv("MUXCORE_GRPC_ADDR"))
	if meshAddr == "" {
		return
	}
	delay := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if m.tryDialCore(meshAddr) {
			m.meshConnected.Store(true)
			slog.Info("health-monitor: connected to core mesh", "addr", meshAddr)
			go m.pollLoop(ctx)
			return
		}
		if err := sleepContext(ctx, delay); err != nil {
			return
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

func (m *Module) tryDialCore(meshAddr string) bool {
	var opts []client.Option
	if os.Getenv("MUXCORE_INSECURE_DISABLE_TLS") == "true" || os.Getenv("MUXCORE_GRPC_INSECURE") == "true" {
		opts = append(opts, client.WithInsecure())
	}
	c, err := client.Dial(meshAddr, opts...)
	if err != nil {
		slog.Warn("health-monitor: dial core failed", "error", err)
		return false
	}
	m.mu.Lock()
	if m.mc != nil {
		_ = m.mc.Close()
	}
	m.mc = c
	if m.meshPub == nil {
		m.meshPub = func(ctx context.Context, eventType, source string, payload []byte) error {
			return c.Events.Publish(ctx, eventType, source, payload)
		}
	}
	m.mu.Unlock()
	return true
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (m *Module) Health(ctx context.Context) error {
	if strings.TrimSpace(os.Getenv("MUXCORE_GRPC_ADDR")) != "" && !m.meshConnected.Load() {
		return fmt.Errorf("not connected to core mesh")
	}
	return nil
}

func (m *Module) ReportHealth(ctx context.Context, req *healthmonitorv1.ReportHealthRequest) (*healthmonitorv1.ReportHealthResponse, error) {
	now := time.Now().UTC()
	var fanout []healthEvent
	m.mu.Lock()
	for _, h := range req.GetModules() {
		id := h.GetModuleId()
		if id == "" {
			continue
		}
		prev, ok := m.moduleHealth[id]
		state := h.GetState()
		if state == "" {
			state = "running"
		}
		entry := &trackedHealth{
			ModuleID: id,
			State:    state,
			Error:    h.GetError(),
			LastSeen: now,
			Stale:    false,
		}
		m.moduleHealth[id] = entry

		if state == "degraded" && (!ok || (prev.State != "degraded" && prev.State != "stale")) {
			m.degradedCount.Add(1)
			slog.Warn("module degraded", "module", id, "error", h.GetError())
			ev := healthEvent{
				EventType: contracts.EventModuleDegraded,
				ModuleID:  id,
				Error:     h.GetError(),
				At:        now,
			}
			m.appendEventLocked(ev)
			fanout = append(fanout, ev)
		}
		if ok && state == "running" && (prev.State == "degraded" || prev.State == "stale" || prev.Stale) {
			slog.Info("module recovered", "module", id)
			ev := healthEvent{
				EventType: contracts.EventModuleRecovered,
				ModuleID:  id,
				At:        now,
			}
			m.appendEventLocked(ev)
			fanout = append(fanout, ev)
		}
	}
	m.mu.Unlock()
	m.processEvents(ctx, fanout)
	return &healthmonitorv1.ReportHealthResponse{Status: "ok"}, nil
}

func (m *Module) PublishEvent(ctx context.Context, req *healthmonitorv1.PublishEventRequest) (*healthmonitorv1.PublishEventResponse, error) {
	m.eventsPublish.Add(1)
	ev := healthEvent{
		EventType: req.GetEventType(),
		ModuleID:  req.GetModuleId(),
		Error:     req.GetMessage(),
		At:        time.Now().UTC(),
	}
	m.mu.Lock()
	m.appendEventLocked(ev)
	m.mu.Unlock()
	slog.Info("health event", "type", ev.EventType, "module", ev.ModuleID, "error", ev.Error)
	m.processEvents(ctx, []healthEvent{ev})
	return &healthmonitorv1.PublishEventResponse{Status: "ok"}, nil
}

func (m *Module) processEvents(ctx context.Context, events []healthEvent) {
	for _, ev := range events {
		m.fanOutMesh(ctx, ev)
		if ev.EventType == contracts.EventModuleDegraded || ev.EventType == contracts.EventModuleStale {
			m.notifyOperator(ctx, ev)
		}
	}
}

func (m *Module) appendEventLocked(ev healthEvent) {
	m.recentEvents = append(m.recentEvents, ev)
	if len(m.recentEvents) > maxRecentEvents {
		m.recentEvents = m.recentEvents[len(m.recentEvents)-maxRecentEvents:]
	}
}

func (m *Module) staleLoop(ctx context.Context) {
	for {
		d := m.getInterval()
		timer := time.NewTimer(d)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			m.markStale()
		}
	}
}

func (m *Module) markStale() {
	cutoff := time.Now().UTC().Add(-2 * m.getInterval())
	var fanout []healthEvent
	m.mu.Lock()
	var stale int64
	for id, h := range m.moduleHealth {
		if h.LastSeen.Before(cutoff) {
			if !h.Stale {
				h.Stale = true
				if h.State != "stale" {
					prev := h.State
					h.State = "stale"
					errMsg := fmt.Sprintf("no report since %s", h.LastSeen.Format(time.RFC3339))
					slog.Warn("module health stale", "module", id, "previous", prev, "last_seen", h.LastSeen)
					ev := healthEvent{
						EventType: contracts.EventModuleStale,
						ModuleID:  id,
						Error:     errMsg,
						At:        time.Now().UTC(),
					}
					m.appendEventLocked(ev)
					fanout = append(fanout, ev)
				}
			}
			stale++
		} else {
			h.Stale = false
		}
	}
	m.staleCount.Store(stale)
	m.mu.Unlock()
	m.processEvents(context.Background(), fanout)
}

func meshEventAllowed(eventType string) bool {
	return strings.HasPrefix(eventType, "module.") || strings.HasPrefix(eventType, "health.")
}

func meshPayloadForEvent(ev healthEvent) ([]byte, error) {
	switch ev.EventType {
	case contracts.EventModuleDegraded:
		return json.Marshal(contracts.ModuleDegradedPayload{
			ModuleID: ev.ModuleID,
			Error:    ev.Error,
		})
	case contracts.EventModuleStale:
		return json.Marshal(contracts.ModuleStalePayload{
			ModuleID: ev.ModuleID,
			Error:    ev.Error,
		})
	case contracts.EventModuleRecovered:
		return json.Marshal(contracts.ModuleRecoveredPayload{
			ModuleID: ev.ModuleID,
		})
	default:
		return json.Marshal(map[string]any{
			"module_id":  ev.ModuleID,
			"error":      ev.Error,
			"event_type": ev.EventType,
			"at":         ev.At.Format(time.RFC3339Nano),
		})
	}
}

func (m *Module) fanOutMesh(ctx context.Context, ev healthEvent) {
	if !meshEventAllowed(ev.EventType) {
		return
	}
	m.mu.Lock()
	pub := m.meshPub
	m.mu.Unlock()
	if pub == nil {
		return
	}
	payload, err := meshPayloadForEvent(ev)
	if err != nil {
		slog.Warn("health-monitor: marshal mesh payload failed", "type", ev.EventType, "error", err)
		return
	}
	src := m.id
	if src == "" {
		src = "health-monitor"
	}
	if err := pub(ctx, ev.EventType, src, payload); err != nil {
		slog.Warn("health-monitor: mesh publish failed", "type", ev.EventType, "module", ev.ModuleID, "error", err)
		return
	}
	m.meshPublishN.Add(1)
	slog.Debug("health-monitor: mesh published", "type", ev.EventType, "module", ev.ModuleID)
}

type statusModule struct {
	ModuleID string `json:"module_id"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
	LastSeen string `json:"last_seen"`
	Stale    bool   `json:"stale"`
}

type statusResponse struct {
	Status        string         `json:"status"`
	Modules       []statusModule `json:"modules"`
	ModuleCount   int            `json:"module_count"`
	DegradedCount int64          `json:"degraded_transitions"`
	StaleCount    int64          `json:"stale_count"`
	EventsPublish int64          `json:"events_published"`
	MeshPublished int64          `json:"mesh_published"`
	RecentEvents  []healthEvent  `json:"recent_events"`
	Interval      string         `json:"interval"`
	StaleAfter    string         `json:"stale_after"`
}

func (m *Module) snapshot() statusResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	mods := make([]statusModule, 0, len(m.moduleHealth))
	running, degraded, stale := 0, 0, 0
	for _, h := range m.moduleHealth {
		mods = append(mods, statusModule{
			ModuleID: h.ModuleID,
			State:    h.State,
			Error:    h.Error,
			LastSeen: h.LastSeen.Format(time.RFC3339Nano),
			Stale:    h.Stale,
		})
		switch {
		case h.Stale || h.State == "stale":
			stale++
		case h.State == "degraded":
			degraded++
		default:
			running++
		}
	}
	events := make([]healthEvent, len(m.recentEvents))
	copy(events, m.recentEvents)

	agg := "ok"
	if degraded > 0 || stale > 0 {
		agg = "degraded"
	}
	if len(mods) == 0 {
		agg = "idle"
	}

	return statusResponse{
		Status:        agg,
		Modules:       mods,
		ModuleCount:   len(mods),
		DegradedCount: m.degradedCount.Load(),
		StaleCount:    int64(stale),
		EventsPublish: m.eventsPublish.Load(),
		MeshPublished: m.meshPublishN.Load(),
		RecentEvents:  events,
		Interval:      m.interval.String(),
		StaleAfter:    (2 * m.interval).String(),
	}
}

func (m *Module) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (m *Module) handleStatus(w http.ResponseWriter, r *http.Request) {
	st := m.snapshot()
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(st)
}

// SnapshotForTest exposes aggregate status for unit tests.
func (m *Module) SnapshotForTest() statusResponse {
	return m.snapshot()
}

// MarkStaleForTest runs one stale sweep (tests).
func (m *Module) MarkStaleForTest() {
	m.markStale()
}

// SetLastSeenForTest backdates a module's last_seen (tests).
func (m *Module) SetLastSeenForTest(moduleID string, t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.moduleHealth[moduleID]; ok {
		h.LastSeen = t
	}
}

// SetMeshPublisherForTest injects a mesh publisher stub.
func (m *Module) SetMeshPublisherForTest(pub meshPublisher) {
	m.mu.Lock()
	m.meshPub = pub
	m.mu.Unlock()
}

// SetMeshConnectedForTest marks mesh connectivity for Health() tests.
func (m *Module) SetMeshConnectedForTest(connected bool) {
	m.meshConnected.Store(connected)
}
