package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	healthmonitorv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/healthmonitor/v1"
)

type Module struct {
	healthmonitorv1.UnimplementedHealthMonitorServiceServer
	mu            sync.Mutex
	moduleHealth  map[string]*healthmonitorv1.ModuleHealth
	eventsPublish atomic.Int64
	degradedCount atomic.Int64

	grpcSrv  *grpc.Server
	grpcLis  net.Listener
	httpLis  net.Listener
	id       string
	grpcAddr string
	httpAddr string
	interval time.Duration
}

type Config struct {
	ID       string
	GRPCAddr string
	HTTPAddr string
	Interval time.Duration
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "health-monitor"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9202"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9203"
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if v := os.Getenv("HEALTH_MONITOR_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	return &Module{
		id:           cfg.ID,
		grpcAddr:     cfg.GRPCAddr,
		httpAddr:     cfg.HTTPAddr,
		interval:     cfg.Interval,
		moduleHealth: make(map[string]*healthmonitorv1.ModuleHealth),
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Health Monitor",
		Version:      "0.1.0",
		Roles:        []string{"infrastructure"},
		Description:  "Aggregated module health monitoring and degradation detection",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityHealthMonitor},
		Contracts: []contracts.ContractDeclaration{
			{Repo: "github.com/Muxcore-Media/core/pkg/contracts", Interface: "HealthMonitorProvider", Version: "v0.4.0"},
		},
		MinCoreVersion: "0.4.0",
		HTTPAddr:       m.grpcAddr,
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
	slog.Info("health-monitor initialized", "grpc", m.grpcAddr, "http", m.httpAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	healthmonitorv1.RegisterHealthMonitorServiceServer(m.grpcSrv, m)
	go func() {
		slog.Info("health-monitor gRPC started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.grpcLis); err != nil {
			slog.Error("health-monitor gRPC error", "error", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	go func() {
		slog.Info("health-monitor HTTP started", "addr", m.httpAddr)
		if err := http.Serve(m.httpLis, mux); err != nil {
			slog.Error("health-monitor HTTP error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	slog.Info("health-monitor stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}

func (m *Module) ReportHealth(ctx context.Context, req *healthmonitorv1.ReportHealthRequest) (*healthmonitorv1.ReportHealthResponse, error) {
	m.mu.Lock()
	for _, h := range req.GetModules() {
		prev, ok := m.moduleHealth[h.ModuleId]
		m.moduleHealth[h.ModuleId] = h
		if ok && prev.State == "running" && h.State == "degraded" {
			m.degradedCount.Add(1)
			slog.Warn("module degraded", "module", h.ModuleId, "error", h.Error)
		}
	}
	m.mu.Unlock()
	return &healthmonitorv1.ReportHealthResponse{Status: "ok"}, nil
}

func (m *Module) PublishEvent(ctx context.Context, req *healthmonitorv1.PublishEventRequest) (*healthmonitorv1.PublishEventResponse, error) {
	m.eventsPublish.Add(1)
	slog.Info("health event", "type", req.EventType, "module", req.ModuleId, "message", req.Message)
	return &healthmonitorv1.PublishEventResponse{Status: "ok"}, nil
}
