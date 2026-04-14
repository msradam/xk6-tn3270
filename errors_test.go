package tn3270

import (
	"errors"
	"strings"
	"testing"

	"go.k6.io/k6/metrics"
)

func TestRegisterMetricsDefinesAllExpected(t *testing.T) {
	reg := metrics.NewRegistry()
	m, err := registerMetrics(reg)
	if err != nil {
		t.Fatalf("registerMetrics: %v", err)
	}
	all := []*metrics.Metric{
		m.ConnectDuration, m.SendDuration, m.WaitDuration, m.SessionDuration,
		m.Errors, m.WaitTimeouts, m.Screens, m.Connects, m.Disconnects,
		m.AIDsSent, m.BytesIn, m.BytesOut,
	}
	for i, metric := range all {
		if metric == nil {
			t.Errorf("metric slot %d is nil", i)
		}
	}
}

func TestTn3270ErrorCode(t *testing.T) {
	err := newError(CodeWaitTimeout, "boom")
	if err.Code != CodeWaitTimeout {
		t.Errorf("expected code %q, got %q", CodeWaitTimeout, err.Code)
	}
	if !strings.Contains(err.Error(), CodeWaitTimeout) {
		t.Errorf("Error() should include code, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("Error() should include message, got %q", err.Error())
	}
}

func TestTn3270ErrorUnwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := wrapError(CodeProtocol, "wrapped", cause)
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is should find wrapped cause")
	}
	if !strings.Contains(err.Error(), "root cause") {
		t.Errorf("Error() should render cause, got %q", err.Error())
	}
}

func TestAsCode(t *testing.T) {
	if got := asCode(newError(CodeNotConnected, "x")); got != CodeNotConnected {
		t.Errorf("asCode returned %q, want %q", got, CodeNotConnected)
	}
	if got := asCode(errors.New("plain")); got != "" {
		t.Errorf("asCode on plain error returned %q, want empty", got)
	}
	// Codes should propagate through wrapError chaining so scripts can branch
	// on the outermost code while preserving the cause.
	inner := newError(CodeNegotiation, "inner")
	outer := wrapError(CodeConnectFailed, "outer", inner)
	if got := asCode(outer); got != CodeConnectFailed {
		t.Errorf("asCode returned %q, want %q", got, CodeConnectFailed)
	}
}

func TestErrorCodeSurfaceStable(t *testing.T) {
	// Scripts match on these strings — any change is a breaking API change.
	want := map[string]string{
		"CodeInvalidArgument":  "invalid_argument",
		"CodeNotConnected":     "not_connected",
		"CodeConnectFailed":    "connect_failed",
		"CodeConnectTimeout":   "connect_timeout",
		"CodeTLSHandshake":     "tls_handshake",
		"CodeNegotiation":      "negotiation_failed",
		"CodeWaitTimeout":      "wait_timeout",
		"CodeHostClosed":       "host_closed",
		"CodeProtocol":         "protocol_error",
		"CodeInitContext":      "init_context",
		"CodeProtectedField":   "protected_field",
		"CodeScreenshotFailed": "screenshot_failed",
	}
	got := map[string]string{
		"CodeInvalidArgument":  CodeInvalidArgument,
		"CodeNotConnected":     CodeNotConnected,
		"CodeConnectFailed":    CodeConnectFailed,
		"CodeConnectTimeout":   CodeConnectTimeout,
		"CodeTLSHandshake":     CodeTLSHandshake,
		"CodeNegotiation":      CodeNegotiation,
		"CodeWaitTimeout":      CodeWaitTimeout,
		"CodeHostClosed":       CodeHostClosed,
		"CodeProtocol":         CodeProtocol,
		"CodeInitContext":      CodeInitContext,
		"CodeProtectedField":   CodeProtectedField,
		"CodeScreenshotFailed": CodeScreenshotFailed,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}
