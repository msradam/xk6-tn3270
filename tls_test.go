package tn3270

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildTLSConfigDefaultsToTLS12(t *testing.T) {
	emu := NewEmulator()
	cfg := emu.buildTLSConfig("mainframe.example.com")
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = 0x%x, want 0x%x", cfg.MinVersion, tls.VersionTLS12)
	}
	if cfg.ServerName != "mainframe.example.com" {
		t.Errorf("ServerName = %q, want host", cfg.ServerName)
	}
	if cfg.InsecureSkipVerify {
		t.Errorf("InsecureSkipVerify must default to false")
	}
}

func TestBuildTLSConfigHonorsOverrides(t *testing.T) {
	emu := NewEmulator()
	emu.tlsInsecure = true
	emu.tlsServerName = "override.example.com"
	emu.tlsMinVersion = tls.VersionTLS13
	cfg := emu.buildTLSConfig("dialed.example.com")
	if cfg.ServerName != "override.example.com" {
		t.Errorf("ServerName = %q, want override", cfg.ServerName)
	}
	if !cfg.InsecureSkipVerify {
		t.Errorf("InsecureSkipVerify should be true")
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = 0x%x, want 1.3", cfg.MinVersion)
	}
}

func TestBuildTLSConfigClonesBase(t *testing.T) {
	emu := NewEmulator()
	base := &tls.Config{
		NextProtos:   []string{"h2"},
		KeyLogWriter: nil,
	}
	emu.tlsBase = base
	cfg := emu.buildTLSConfig("h")
	// Clone must surface base fields — here k6 would plumb NextProtos etc.
	if len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != "h2" {
		t.Errorf("base clone dropped NextProtos: %v", cfg.NextProtos)
	}
	// But modifications on the clone must not leak into the base.
	cfg.ServerName = "mutated"
	if base.ServerName == "mutated" {
		t.Errorf("mutation on clone leaked to base tls.Config")
	}
}

func TestParseTLSOptionsMinVersion(t *testing.T) {
	tests := []struct {
		in   string
		want uint16
	}{
		{"1.2", tls.VersionTLS12},
		{"TLSv1.2", tls.VersionTLS12},
		{"1.3", tls.VersionTLS13},
		{"TLSv1.3", tls.VersionTLS13},
	}
	for _, tt := range tests {
		opts, _, err := parseTLSOptions(map[string]interface{}{"minVersion": tt.in})
		if err != nil {
			t.Errorf("%q: unexpected err: %v", tt.in, err)
			continue
		}
		if opts.minVersion != tt.want {
			t.Errorf("%q: got 0x%x, want 0x%x", tt.in, opts.minVersion, tt.want)
		}
	}
}

func TestParseTLSOptionsRejectsBadMinVersion(t *testing.T) {
	_, _, err := parseTLSOptions(map[string]interface{}{"minVersion": "1.1"})
	if err == nil {
		t.Fatal("expected error for disallowed minVersion 1.1")
	}
	if asCode(err) != CodeInvalidArgument {
		t.Errorf("expected %s, got %s", CodeInvalidArgument, asCode(err))
	}
}

func TestParseTLSOptionsRejectsPartialClientCert(t *testing.T) {
	_, _, err := parseTLSOptions(map[string]interface{}{"clientCert": "-----BEGIN CERTIFICATE-----\nzzz\n-----END CERTIFICATE-----"})
	if err == nil || asCode(err) != CodeInvalidArgument {
		t.Fatalf("expected invalid_argument when clientKey missing, got %v", err)
	}
}

func TestParseTLSOptionsTimeoutClamp(t *testing.T) {
	_, _, err := parseTLSOptions(map[string]interface{}{"timeout": 0})
	if err == nil || asCode(err) != CodeInvalidArgument {
		t.Errorf("expected invalid_argument for zero timeout, got %v", err)
	}
	_, _, err = parseTLSOptions(map[string]interface{}{"timeout": 301})
	if err == nil || asCode(err) != CodeInvalidArgument {
		t.Errorf("expected invalid_argument for overlarge timeout, got %v", err)
	}
}

// generateSelfSignedPEMs produces an in-memory cert and private key suitable
// for tls.X509KeyPair; lets the TLS-options parser be tested end-to-end
// without touching the filesystem for fixtures.
func generateSelfSignedPEMs(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

func TestParseTLSOptionsInlinePEM(t *testing.T) {
	certPEM, keyPEM := generateSelfSignedPEMs(t)
	opts, _, err := parseTLSOptions(map[string]interface{}{
		"caCert":     string(certPEM),
		"clientCert": string(certPEM),
		"clientKey":  string(keyPEM),
	})
	if err != nil {
		t.Fatalf("parseTLSOptions: %v", err)
	}
	if opts.rootCAs == nil {
		t.Error("expected rootCAs populated from inline PEM")
	}
	if len(opts.clientCerts) != 1 {
		t.Errorf("expected 1 client cert, got %d", len(opts.clientCerts))
	}
}

func TestParseTLSOptionsFilePEM(t *testing.T) {
	certPEM, keyPEM := generateSelfSignedPEMs(t)
	dir := t.TempDir()
	certPath := filepath.Join(dir, "c.pem")
	keyPath := filepath.Join(dir, "k.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	opts, _, err := parseTLSOptions(map[string]interface{}{
		"caCert":     certPath,
		"clientCert": certPath,
		"clientKey":  keyPath,
	})
	if err != nil {
		t.Fatalf("parseTLSOptions: %v", err)
	}
	if opts.rootCAs == nil {
		t.Error("expected rootCAs from file path")
	}
	if len(opts.clientCerts) != 1 {
		t.Errorf("expected 1 client cert, got %d", len(opts.clientCerts))
	}
}

func TestParseCipherSuitesString(t *testing.T) {
	list, err := parseCipherSuites("ECDHE-RSA-AES128-GCM-SHA256")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(list) != 1 || list[0] != tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 {
		t.Errorf("unexpected list: %v", list)
	}
}

func TestParseCipherSuitesArray(t *testing.T) {
	v := []interface{}{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-CHACHA20-POLY1305"}
	list, err := parseCipherSuites(v)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 suites, got %d", len(list))
	}
}

func TestParseCipherSuitesRejectsUnknown(t *testing.T) {
	_, err := parseCipherSuites([]interface{}{"RSA-RC4-MD5"})
	if err == nil || asCode(err) != CodeInvalidArgument {
		t.Fatalf("expected invalid_argument for unknown suite, got %v", err)
	}
}

func TestParseCipherSuitesRejectsWeakSuitesImplicitly(t *testing.T) {
	// The allow-list deliberately excludes non-PFS RSA_WITH_AES suites and all
	// CBC/RC4 variants; pinning the map membership here prevents a future edit
	// from silently re-admitting them.
	for _, bad := range []string{
		"RSA-AES128-GCM-SHA256", "RSA-AES256-GCM-SHA384",
		"ECDHE-RSA-AES128-SHA", "ECDHE-RSA-AES256-SHA",
		"RSA-RC4-SHA", "ECDHE-RSA-RC4-SHA",
	} {
		if _, ok := tlsCipherSuiteByName[bad]; ok {
			t.Errorf("weak suite %q is in allow-list", bad)
		}
	}
}

func TestBuildTLSConfigAppliesCipherSuites(t *testing.T) {
	emu := NewEmulator()
	emu.tlsCipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}
	cfg := emu.buildTLSConfig("h")
	if len(cfg.CipherSuites) != 1 || cfg.CipherSuites[0] != tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 {
		t.Errorf("CipherSuites not applied: %v", cfg.CipherSuites)
	}
}

func TestProxyDialerAcceptsSocks5(t *testing.T) {
	d, err := proxyDialer("socks5://127.0.0.1:1080", &net.Dialer{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d == nil {
		t.Fatal("nil dialer returned")
	}
}

func TestProxyDialerRejectsUnsupportedScheme(t *testing.T) {
	for _, bad := range []string{
		"http://proxy:8080",
		"https://proxy:8080",
		"socks4://proxy:1080",
		"://nope",
	} {
		_, err := proxyDialer(bad, &net.Dialer{})
		if err == nil {
			t.Errorf("expected error for %q, got nil", bad)
		}
	}
}

func TestParseTLSOptionsRejectsBadProxy(t *testing.T) {
	_, _, err := parseTLSOptions(map[string]interface{}{"proxy": "http://nope:1234"})
	if err == nil || asCode(err) != CodeInvalidArgument {
		t.Fatalf("expected invalid_argument for non-SOCKS proxy, got %v", err)
	}
}

func TestLoadCertPoolRejectsGarbage(t *testing.T) {
	_, err := loadCertPool("not PEM at all")
	if err == nil {
		t.Fatal("expected error for non-PEM input")
	}
	if !strings.Contains(err.Error(), "no such file") && !strings.Contains(err.Error(), "no PEM") {
		t.Errorf("unexpected error: %v", err)
	}
}
