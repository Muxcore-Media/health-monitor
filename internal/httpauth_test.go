package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultHTTPAddrLoopback(t *testing.T) {
	t.Setenv("HEALTH_MONITOR_HTTP_ADDR", "")
	if m := NewModule(Config{}); m.httpAddr != "127.0.0.1:9203" {
		t.Fatalf("default=%q", m.httpAddr)
	}
}

func TestInitNonLoopbackWithoutTokenFails(t *testing.T) {
	t.Setenv("HEALTH_MONITOR_HTTP_TOKEN", "")
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: ":0", Interval: time.Hour})
	err := m.Init(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HEALTH_MONITOR_HTTP_TOKEN") {
		t.Fatalf("expected token error, got %v", err)
	}
}

func TestInitNonLoopbackWithTokenEnv(t *testing.T) {
	t.Setenv("MUXCORE_INSECURE_DISABLE_TLS", "true")
	t.Setenv("HEALTH_MONITOR_HTTP_TOKEN", "s3cret")
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "0.0.0.0:0", Interval: time.Hour})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	_ = m.grpcLis.Close()
	_ = m.httpLis.Close()
}

func TestHTTPHandlerAuth(t *testing.T) {
	m := NewModule(Config{HTTPToken: "s3cret", Interval: time.Hour})
	h := m.httpHandler()
	do := func(path, auth string) int {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if c := do("/health", ""); c != 200 {
		t.Fatalf("health: %d", c)
	}
	if c := do("/status", ""); c != 401 {
		t.Fatalf("status no token: %d", c)
	}
	if c := do("/status", "Bearer nope"); c != 401 {
		t.Fatalf("status wrong token: %d", c)
	}
	if c := do("/status", "Bearer s3cret"); c != 200 {
		t.Fatalf("status ok: %d", c)
	}
}

func TestHTTPHandlerLoopbackNoToken(t *testing.T) {
	m := NewModule(Config{Interval: time.Hour})
	r := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	m.httpHandler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status: %d", w.Code)
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	for a, want := range map[string]bool{"127.0.0.1:1": true, "[::1]:1": true, "localhost:1": true, ":1": false, "0.0.0.0:1": false, "[::]:1": false, "10.0.0.1:1": false} {
		if isLoopbackAddr(a) != want {
			t.Errorf("%s want %v", a, want)
		}
	}
}
