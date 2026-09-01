package internal

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	notificationv1 "github.com/Muxcore-Media/contracts-notification/muxcore/notification/v1"
	"github.com/Muxcore-Media/core/pkg/contracts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func (m *Module) notifyOperator(ctx context.Context, ev healthEvent) {
	switch ev.EventType {
	case contracts.EventModuleDegraded, contracts.EventModuleStale:
	default:
		return
	}
	addr, err := m.findCapabilityAddr(ctx, "notification")
	if err != nil {
		slog.Debug("health-monitor: notification capability missing", "error", err)
		return
	}
	title := "Module health alert"
	if ev.EventType == contracts.EventModuleStale {
		title = "Module health stale"
	}
	msg := ev.Error
	if msg == "" {
		msg = ev.EventType
	}
	notifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Debug("health-monitor: dial notification", "error", err)
		return
	}
	defer func() { _ = conn.Close() }()
	cli := notificationv1.NewNotificationServiceClient(conn)
	_, err = cli.Notify(notifyCtx, &notificationv1.NotifyRequest{
		Title:        title,
		Message:      msg,
		Severity:     notificationv1.Severity_SEVERITY_WARNING,
		SourceModule: m.id,
		Fields: map[string]string{
			"module_id":  ev.ModuleID,
			"event_type": ev.EventType,
		},
	})
	if err != nil {
		slog.Debug("health-monitor: notify failed", "module", ev.ModuleID, "error", err)
	}
}

func (m *Module) findCapabilityAddr(ctx context.Context, capability string) (string, error) {
	mc := m.meshClient()
	if mc == nil {
		return "", fmt.Errorf("not connected to core")
	}
	modules, err := mc.Discovery.FindByCapability(ctx, capability)
	if err != nil {
		return "", err
	}
	for _, mod := range modules {
		addr := dialAddrForModule(mod.Id, mod.HttpAddr)
		if addr != "" {
			return addr, nil
		}
	}
	return "", fmt.Errorf("no %s module found", capability)
}
