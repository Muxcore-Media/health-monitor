package grpctls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const moduleCN = "health-monitor"

// InsecureAllowed reports whether plaintext gRPC is explicitly allowed (dev only).
func InsecureAllowed() bool {
	for _, key := range []string{"MUXCORE_INSECURE_DISABLE_TLS", "MUXCORE_GRPC_INSECURE"} {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if v == "true" || v == "1" {
			return true
		}
	}
	return false
}

// ResolveListenAddr rewrites wildcard listen addresses to loopback when insecure dev mode is on.
func ResolveListenAddr(addr string) string {
	if !InsecureAllowed() {
		return addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return "127.0.0.1" + addr
		}
		return addr
	}
	if host == "" || host == "0.0.0.0" {
		return "127.0.0.1:" + port
	}
	return addr
}

// ServerConfig returns TLS settings for the module gRPC listener.
// Returns (nil, nil) when MUXCORE_INSECURE_DISABLE_TLS (or MUXCORE_GRPC_INSECURE) is set.
func ServerConfig() (*tls.Config, error) {
	if InsecureAllowed() {
		return nil, nil
	}

	certFile := envFirst("HEALTH_MONITOR_TLS_CERT", "MUXCORE_TLS_CERT")
	keyFile := envFirst("HEALTH_MONITOR_TLS_KEY", "MUXCORE_TLS_KEY")
	caFile := envFirst("HEALTH_MONITOR_TLS_CA", "MUXCORE_TLS_CA")

	if certFile == "" || keyFile == "" {
		dir := envFirst("HEALTH_MONITOR_TLS_DIR")
		if dir == "" {
			dir = defaultTLSDir()
		}
		var genErr error
		certFile, keyFile, genErr = ensureAutoCert(dir)
		if genErr != nil {
			return nil, genErr
		}
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS cert/key: %w", err)
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	if caFile != "" {
		pemBytes, err := os.ReadFile(caFile) //nolint:gosec // operator-configured CA path
		if err != nil {
			return nil, fmt.Errorf("read TLS CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("parse TLS CA from %q", caFile)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg, nil
}

func envFirst(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func defaultTLSDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "muxcore", "tls", "health-monitor")
	}
	return filepath.Join(home, ".muxcore", "tls", "health-monitor")
}

func ensureAutoCert(dir string) (certFile, keyFile string, err error) {
	certFile = filepath.Join(dir, "server.crt")
	keyFile = filepath.Join(dir, "server.key")
	if filePairExists(certFile, keyFile) {
		return certFile, keyFile, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("create TLS dir %s: %w", dir, err)
	}
	if err := generateServerCert(certFile, keyFile); err != nil {
		return "", "", err
	}
	return certFile, keyFile, nil
}

func filePairExists(certFile, keyFile string) bool {
	if _, err := os.Stat(certFile); err != nil {
		return false
	}
	if _, err := os.Stat(keyFile); err != nil {
		return false
	}
	return true
}

func generateServerCert(certFile, keyFile string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate TLS key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate TLS serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   moduleCN,
			Organization: []string{"MuxCore"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create TLS certificate: %w", err)
	}

	certOut, err := os.OpenFile(certFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // public cert
	if err != nil {
		return fmt.Errorf("write TLS cert: %w", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		_ = certOut.Close()
		return fmt.Errorf("encode TLS cert: %w", err)
	}
	if err := certOut.Close(); err != nil {
		return fmt.Errorf("close TLS cert: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal TLS key: %w", err)
	}
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write TLS key: %w", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		_ = keyOut.Close()
		return fmt.Errorf("encode TLS key: %w", err)
	}
	if err := keyOut.Close(); err != nil {
		return fmt.Errorf("close TLS key: %w", err)
	}

	return nil
}
