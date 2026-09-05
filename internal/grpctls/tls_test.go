package grpctls

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInsecureAllowed(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{
			name: "unset",
			env:  map[string]string{"MUXCORE_INSECURE_DISABLE_TLS": "", "MUXCORE_GRPC_INSECURE": ""},
			want: false,
		},
		{
			name: "MUXCORE_INSECURE_DISABLE_TLS true",
			env:  map[string]string{"MUXCORE_INSECURE_DISABLE_TLS": "true"},
			want: true,
		},
		{
			name: "MUXCORE_INSECURE_DISABLE_TLS 1",
			env:  map[string]string{"MUXCORE_INSECURE_DISABLE_TLS": "1"},
			want: true,
		},
		{
			name: "MUXCORE_GRPC_INSECURE true",
			env:  map[string]string{"MUXCORE_GRPC_INSECURE": "true"},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MUXCORE_INSECURE_DISABLE_TLS", tt.env["MUXCORE_INSECURE_DISABLE_TLS"])
			t.Setenv("MUXCORE_GRPC_INSECURE", tt.env["MUXCORE_GRPC_INSECURE"])
			if got := InsecureAllowed(); got != tt.want {
				t.Fatalf("InsecureAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveListenAddr(t *testing.T) {
	t.Setenv("MUXCORE_INSECURE_DISABLE_TLS", "true")
	if got := ResolveListenAddr(":9202"); got != "127.0.0.1:9202" {
		t.Fatalf("ResolveListenAddr(:9202) = %q", got)
	}
	if got := ResolveListenAddr("0.0.0.0:9202"); got != "127.0.0.1:9202" {
		t.Fatalf("ResolveListenAddr(0.0.0.0:9202) = %q", got)
	}
	t.Setenv("MUXCORE_INSECURE_DISABLE_TLS", "")
	if got := ResolveListenAddr(":9202"); got != ":9202" {
		t.Fatalf("secure ResolveListenAddr(:9202) = %q", got)
	}
}

func TestServerConfigInsecure(t *testing.T) {
	t.Setenv("MUXCORE_INSECURE_DISABLE_TLS", "true")
	cfg, err := ServerConfig()
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil TLS config in insecure mode, got %#v", cfg)
	}
}

func TestServerConfigAutoCert(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MUXCORE_INSECURE_DISABLE_TLS", "")
	t.Setenv("HEALTH_MONITOR_TLS_DIR", dir)
	t.Setenv("HEALTH_MONITOR_TLS_CERT", "")
	t.Setenv("HEALTH_MONITOR_TLS_KEY", "")
	t.Setenv("MUXCORE_TLS_CERT", "")
	t.Setenv("MUXCORE_TLS_KEY", "")

	cfg, err := ServerConfig()
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected TLS config")
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("expected one certificate, got %d", len(cfg.Certificates))
	}
	if !filePairExists(filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")) {
		t.Fatal("expected auto-generated cert files")
	}
}

func TestServerConfigFromEnv(t *testing.T) {
	dir := t.TempDir()
	autoDir := t.TempDir()
	t.Setenv("MUXCORE_INSECURE_DISABLE_TLS", "")
	t.Setenv("HEALTH_MONITOR_TLS_DIR", autoDir)
	if err := generateServerCert(filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")); err != nil {
		t.Fatalf("generateServerCert: %v", err)
	}
	t.Setenv("HEALTH_MONITOR_TLS_CERT", filepath.Join(dir, "server.crt"))
	t.Setenv("HEALTH_MONITOR_TLS_KEY", filepath.Join(dir, "server.key"))

	cfg, err := ServerConfig()
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	if cfg == nil || len(cfg.Certificates) != 1 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if _, err := os.Stat(filepath.Join(autoDir, "server.crt")); err == nil {
		t.Fatal("should not auto-generate when env cert paths are set")
	}
}
