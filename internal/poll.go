package internal

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	discoveryv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/discovery/v1"
	healthmonitorv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/healthmonitor/v1"
)

func (m *Module) pollLoop(ctx context.Context) {
	for {
		d := m.getInterval()
		timer := time.NewTimer(d)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			m.pollOnce(ctx)
		}
	}
}

func (m *Module) pollOnce(ctx context.Context) {
	mc := m.meshClient()
	if mc == nil {
		return
	}
	pollCtx, cancel := context.WithTimeout(ctx, m.getInterval())
	defer cancel()
	resp, err := mc.Discovery.Raw().ListAll(pollCtx, &discoveryv1.ListAllRequest{})
	if err != nil {
		slog.Debug("health-monitor: ListAll failed", "error", err)
		return
	}
	var modules []*healthmonitorv1.ModuleHealth
	for _, entry := range resp.GetEntries() {
		info := entry.GetInfo()
		if info == nil {
			continue
		}
		moduleID := info.GetId()
		if moduleID == "" || moduleID == m.id {
			continue
		}
		httpAddr := strings.TrimSpace(info.GetHttpAddr())
		if httpAddr == "" {
			continue
		}
		state, errMsg := m.probeModuleHealth(pollCtx, moduleID, httpAddr)
		modules = append(modules, &healthmonitorv1.ModuleHealth{
			ModuleId: moduleID,
			State:    state,
			Error:    errMsg,
		})
	}
	if len(modules) == 0 {
		return
	}
	if _, err := m.ReportHealth(pollCtx, &healthmonitorv1.ReportHealthRequest{Modules: modules}); err != nil {
		slog.Debug("health-monitor: active poll ReportHealth failed", "error", err)
	}
}

func (m *Module) probeModuleHealth(ctx context.Context, moduleID, httpAddr string) (state, errMsg string) {
	url := healthURLForModule(moduleID, httpAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "degraded", err.Error()
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "degraded", err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return "degraded", fmt.Sprintf("HTTP %d: %s", resp.StatusCode, msg)
	}
	return "running", ""
}

func healthURLForModule(moduleID, httpAddr string) string {
	base := dialAddrForModule(moduleID, httpAddr)
	if strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://") {
		return strings.TrimRight(base, "/") + "/health"
	}
	return "http://" + strings.TrimRight(base, "/") + "/health"
}
