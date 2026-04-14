package tn3270

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/metrics"
	"golang.org/x/net/proxy"
)

type logrusAdapter struct {
	vu modules.VU
}

func (l *logrusAdapter) Printf(format string, args ...interface{}) {
	if state := l.vu.State(); state != nil {
		state.Logger.Infof(format, args...)
	} else if initEnv := l.vu.InitEnv(); initEnv != nil {
		initEnv.Logger.Infof(format, args...)
	}
}

type Client struct {
	vu           modules.VU
	emu          *Emulator
	mu           sync.Mutex
	connected    bool
	model        int
	codePage     *CodePage
	trace        bool
	metrics      *tn3270Metrics
	sessionStart time.Time
	// Last-observed emulator counter values; metrics emit the delta per op.
	bytesInSent  int64
	bytesOutSent int64
	// Non-nil while a connection is live; closed by Disconnect to stop the janitor.
	janitorStop chan struct{}
}

func NewClient(vu modules.VU, m *tn3270Metrics) *Client {
	return &Client{
		vu:      vu,
		metrics: m,
	}
}

func (c *Client) newEmulator() *Emulator {
	var emu *Emulator
	if c.model > 0 {
		emu = NewEmulatorModel(c.model)
	} else {
		emu = NewEmulator()
	}
	if c.codePage != nil {
		emu.codePage = c.codePage
	}
	emu.trace = c.trace
	emu.logger = &logrusAdapter{vu: c.vu}
	return emu
}

func (c *Client) pushMetric(metric *metrics.Metric, value float64) {
	if c.metrics == nil || metric == nil {
		return
	}
	state := c.vu.State()
	if state == nil {
		return
	}
	ctx := c.vu.Context()
	now := time.Now()
	metrics.PushIfNotDone(ctx, state.Samples, metrics.Sample{
		TimeSeries: metrics.TimeSeries{
			Metric: metric,
			Tags:   state.Tags.GetCurrentValues().Tags,
		},
		Time:  now,
		Value: value,
	})
}

// Emit accumulated byte counts since the last call. Cheaper than a sample per byte.
func (c *Client) flushBytes() {
	if c.emu == nil {
		return
	}
	in := c.emu.BytesIn()
	out := c.emu.BytesOut()
	if din := in - c.bytesInSent; din > 0 {
		c.pushMetric(c.metrics.BytesIn, float64(din))
		c.bytesInSent = in
	}
	if dout := out - c.bytesOutSent; dout > 0 {
		c.pushMetric(c.metrics.BytesOut, float64(dout))
		c.bytesOutSent = out
	}
}

func (c *Client) SetTrace(enabled bool) {
	c.mu.Lock()
	c.trace = enabled
	if c.emu != nil {
		c.emu.trace = enabled
	}
	c.mu.Unlock()
}

func (c *Client) checkConnected() error {
	if c.emu == nil || !c.connected {
		return newError(CodeNotConnected, "not connected")
	}
	return nil
}

// vu.State() is nil during k6's init phase, when network I/O is not permitted.
// A nil vu itself is permitted only because unit tests construct Clients without one.
func (c *Client) checkInitContext() error {
	if c.vu == nil {
		return nil
	}
	if c.vu.State() == nil {
		return newError(CodeInitContext, "TN3270 connections are not supported in the init context")
	}
	return nil
}

func (c *Client) Connect(host string, port int, timeout ...int) error {
	if host == "" {
		return newError(CodeInvalidArgument, "host cannot be empty")
	}
	if len(host) > 253 {
		return newError(CodeInvalidArgument, "host exceeds maximum length of 253 characters")
	}
	if port < 1 || port > 65535 {
		return newError(CodeInvalidArgument, fmt.Sprintf("port must be between 1 and 65535, got %d", port))
	}

	timeoutSec := 30
	if len(timeout) > 0 && timeout[0] > 0 {
		timeoutSec = timeout[0]
	}
	if timeoutSec < 1 || timeoutSec > 300 {
		return newError(CodeInvalidArgument, fmt.Sprintf("timeout must be between 1 and 300 seconds, got %d", timeoutSec))
	}

	if err := c.checkInitContext(); err != nil {
		return err
	}
	return c.doConnect(host, port, time.Duration(timeoutSec)*time.Second, false, nil)
}

// Default TLS path: MinVersion 1.2, host as SNI. Use ConnectTLSWithOptions for
// CA bundles, client certs, cipher policy, or proxy.
func (c *Client) ConnectTLS(host string, port int, insecure bool, timeout ...int) error {
	if host == "" {
		return newError(CodeInvalidArgument, "host cannot be empty")
	}
	if len(host) > 253 {
		return newError(CodeInvalidArgument, "host exceeds maximum length of 253 characters")
	}
	if port < 1 || port > 65535 {
		return newError(CodeInvalidArgument, fmt.Sprintf("port must be between 1 and 65535, got %d", port))
	}

	timeoutSec := 30
	if len(timeout) > 0 && timeout[0] > 0 {
		timeoutSec = timeout[0]
	}
	if timeoutSec < 1 || timeoutSec > 300 {
		return newError(CodeInvalidArgument, fmt.Sprintf("timeout must be between 1 and 300 seconds, got %d", timeoutSec))
	}

	if err := c.checkInitContext(); err != nil {
		return err
	}
	opts := &tlsSetup{insecure: insecure}
	return c.doConnect(host, port, time.Duration(timeoutSec)*time.Second, true, opts)
}

// Option keys: insecure (bool), serverName (string), minVersion ("1.2"|"1.3"),
// caCert / clientCert / clientKey (PEM string or filesystem path),
// cipherSuites (string or array of allow-listed names),
// proxy ("socks5://..." or "socks5h://..."), timeout (seconds).
func (c *Client) ConnectTLSWithOptions(host string, port int, options map[string]interface{}) error {
	if host == "" {
		return newError(CodeInvalidArgument, "host cannot be empty")
	}
	if len(host) > 253 {
		return newError(CodeInvalidArgument, "host exceeds maximum length of 253 characters")
	}
	if port < 1 || port > 65535 {
		return newError(CodeInvalidArgument, fmt.Sprintf("port must be between 1 and 65535, got %d", port))
	}

	opts, timeoutSec, err := parseTLSOptions(options)
	if err != nil {
		return err
	}
	if err := c.checkInitContext(); err != nil {
		return err
	}
	return c.doConnect(host, port, time.Duration(timeoutSec)*time.Second, true, opts)
}

type tlsSetup struct {
	insecure     bool
	serverName   string
	minVersion   uint16
	rootCAs      *x509.CertPool
	clientCerts  []tls.Certificate
	cipherSuites []uint16
	proxyURL     string
}

// AEAD + forward-secrecy suites only. Non-PFS RSA, CBC, and RC4 are excluded
// by design. TLS 1.3 suites are always enabled and not selectable.
var tlsCipherSuiteByName = map[string]uint16{
	"ECDHE-ECDSA-AES128-GCM-SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-ECDSA-AES256-GCM-SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-ECDSA-CHACHA20-POLY1305": tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	"ECDHE-RSA-AES128-GCM-SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-RSA-AES256-GCM-SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-RSA-CHACHA20-POLY1305":   tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
}

func parseTLSOptions(options map[string]interface{}) (*tlsSetup, int, error) {
	opts := &tlsSetup{}
	timeoutSec := 30

	if v, ok := options["insecure"]; ok {
		if b, ok := v.(bool); ok {
			opts.insecure = b
		}
	}
	if v, ok := options["serverName"]; ok {
		if s, ok := v.(string); ok {
			opts.serverName = s
		}
	}
	if v, ok := options["minVersion"]; ok {
		if s, ok := v.(string); ok {
			switch s {
			case "1.2", "TLSv1.2":
				opts.minVersion = tls.VersionTLS12
			case "1.3", "TLSv1.3":
				opts.minVersion = tls.VersionTLS13
			default:
				return nil, 0, newError(CodeInvalidArgument, fmt.Sprintf("unsupported minVersion %q (use \"1.2\" or \"1.3\")", s))
			}
		}
	}
	if v, ok := options["caCert"]; ok {
		if s, ok := v.(string); ok && s != "" {
			pool, err := loadCertPool(s)
			if err != nil {
				return nil, 0, wrapError(CodeInvalidArgument, "failed to load CA cert", err)
			}
			opts.rootCAs = pool
		}
	}

	var clientCertPEM, clientKeyPEM string
	if v, ok := options["clientCert"]; ok {
		if s, ok := v.(string); ok {
			clientCertPEM = s
		}
	}
	if v, ok := options["clientKey"]; ok {
		if s, ok := v.(string); ok {
			clientKeyPEM = s
		}
	}
	if clientCertPEM != "" || clientKeyPEM != "" {
		if clientCertPEM == "" || clientKeyPEM == "" {
			return nil, 0, newError(CodeInvalidArgument, "clientCert and clientKey must both be provided")
		}
		certBytes, err := readPEMOrFile(clientCertPEM)
		if err != nil {
			return nil, 0, wrapError(CodeInvalidArgument, "failed to read clientCert", err)
		}
		keyBytes, err := readPEMOrFile(clientKeyPEM)
		if err != nil {
			return nil, 0, wrapError(CodeInvalidArgument, "failed to read clientKey", err)
		}
		cert, err := tls.X509KeyPair(certBytes, keyBytes)
		if err != nil {
			return nil, 0, wrapError(CodeInvalidArgument, "invalid client certificate or key", err)
		}
		opts.clientCerts = []tls.Certificate{cert}
	}

	if v, ok := options["cipherSuites"]; ok {
		list, err := parseCipherSuites(v)
		if err != nil {
			return nil, 0, err
		}
		opts.cipherSuites = list
	}
	if v, ok := options["proxy"]; ok {
		if s, ok := v.(string); ok && s != "" {
			if _, err := proxyDialer(s, &net.Dialer{}); err != nil {
				return nil, 0, wrapError(CodeInvalidArgument, "invalid proxy URL", err)
			}
			opts.proxyURL = s
		}
	}

	if v, ok := options["timeout"]; ok {
		switch n := v.(type) {
		case int:
			timeoutSec = n
		case int64:
			timeoutSec = int(n)
		case float64:
			timeoutSec = int(n)
		}
	}
	if timeoutSec < 1 || timeoutSec > 300 {
		return nil, 0, newError(CodeInvalidArgument, fmt.Sprintf("timeout must be between 1 and 300 seconds, got %d", timeoutSec))
	}

	return opts, timeoutSec, nil
}

// Reject (don't silently drop) unknown suites — silent drops would mislead
// callers about which policy is actually in force.
func parseCipherSuites(v interface{}) ([]uint16, error) {
	var names []string
	switch x := v.(type) {
	case string:
		names = []string{x}
	case []interface{}:
		for _, elem := range x {
			if s, ok := elem.(string); ok {
				names = append(names, s)
			}
		}
	case []string:
		names = x
	default:
		return nil, newError(CodeInvalidArgument, "cipherSuites must be a string or array of strings")
	}
	out := make([]uint16, 0, len(names))
	for _, n := range names {
		id, ok := tlsCipherSuiteByName[n]
		if !ok {
			return nil, newError(CodeInvalidArgument, fmt.Sprintf("unsupported cipher suite %q", n))
		}
		out = append(out, id)
	}
	return out, nil
}

func proxyDialer(proxyURL string, forward proxy.Dialer) (proxy.ContextDialer, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "socks5", "socks5h":
		// supported
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q (only socks5/socks5h)", u.Scheme)
	}
	d, err := proxy.FromURL(u, forward)
	if err != nil {
		return nil, err
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		// SOCKS5 has implemented ContextDialer since Go 1.12; guards a future scheme.
		return nil, fmt.Errorf("proxy dialer for %q does not support context cancellation", u.Scheme)
	}
	return cd, nil
}

// dialerToProxyDialer satisfies the legacy proxy.Dialer interface (no ctx)
// using a dialContexter underneath. ctx is dropped only for the proxy hop
// itself; the tunneled dial preserves ctx through the SOCKS chain.
type dialerToProxyDialer struct{ d dialContexter }

func (a dialerToProxyDialer) Dial(network, addr string) (net.Conn, error) {
	return a.d.DialContext(context.Background(), network, addr)
}

// Accepts either inline PEM (begins with "-----BEGIN") or a filesystem path.
func readPEMOrFile(s string) ([]byte, error) {
	if strings.HasPrefix(strings.TrimSpace(s), "-----BEGIN") {
		return []byte(s), nil
	}
	return os.ReadFile(s) //#nosec G304 -- user-controlled path is expected
}

func loadCertPool(s string) (*x509.CertPool, error) {
	data, err := readPEMOrFile(s)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		// AppendCertsFromPEM silently skips bad blocks; surface a clear error.
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("no PEM blocks found")
		}
		return nil, fmt.Errorf("no usable certificates in PEM data")
	}
	return pool, nil
}

func (c *Client) doConnect(host string, port int, timeout time.Duration, useTLS bool, opts *tlsSetup) error {
	c.mu.Lock()
	emu := c.newEmulator()
	if useTLS {
		emu.useTLS = true
		if opts != nil {
			emu.tlsInsecure = opts.insecure
			emu.tlsServerName = opts.serverName
			emu.tlsMinVersion = opts.minVersion
			emu.tlsRootCAs = opts.rootCAs
			emu.tlsClientCrts = opts.clientCerts
			emu.tlsCipherSuites = opts.cipherSuites
		}
		if state := c.vu.State(); state != nil && state.TLSConfig != nil {
			emu.tlsBase = state.TLSConfig
		}
	}
	// k6 runtime network policies (--blocked-hostnames, DNS, Hosts map) apply
	// because we dial through state.Dialer.
	if state := c.vu.State(); state != nil && state.Dialer != nil {
		emu.dialer = state.Dialer
	}
	// Chain proxy off whichever dialer we have so the proxy hop also obeys policy.
	if opts != nil && opts.proxyURL != "" {
		var forward proxy.Dialer = &net.Dialer{}
		if emu.dialer != nil {
			forward = dialerToProxyDialer{emu.dialer}
		}
		cd, err := proxyDialer(opts.proxyURL, forward)
		if err != nil {
			c.mu.Unlock()
			return wrapError(CodeInvalidArgument, "failed to set up proxy", err)
		}
		emu.dialer = cd
	}
	c.emu = emu
	c.bytesInSent = 0
	c.bytesOutSent = 0
	c.mu.Unlock()

	ctx := c.vu.Context()
	start := time.Now()

	if err := emu.Connect(ctx, host, port, timeout); err != nil {
		c.flushBytes()
		c.pushMetric(c.metrics.Errors, 1)
		c.mu.Lock()
		c.emu = nil
		c.mu.Unlock()
		return classifyConnectError(host, port, err)
	}

	c.pushMetric(c.metrics.ConnectDuration, float64(time.Since(start).Milliseconds()))
	c.pushMetric(c.metrics.Connects, 1)
	c.flushBytes()

	c.mu.Lock()
	c.connected = true
	c.sessionStart = time.Now()
	// If the script forgets disconnect(), VU ctx cancellation closes the socket
	// at iteration boundary instead of waiting on Go's GC.
	stop := make(chan struct{})
	c.janitorStop = stop
	vuCtx := c.vu.Context()
	go func() {
		select {
		case <-vuCtx.Done():
			_ = c.Disconnect()
		case <-stop:
		}
	}()
	c.mu.Unlock()

	return nil
}

func classifyConnectError(host string, port int, err error) error {
	msg := err.Error()
	code := CodeConnectFailed
	switch {
	case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "timeout during initial handshake"), strings.Contains(msg, "deadline exceeded"):
		code = CodeConnectTimeout
	case strings.Contains(msg, "x509"), strings.Contains(msg, "tls:"), strings.Contains(msg, "certificate"):
		code = CodeTLSHandshake
	case strings.Contains(msg, "failed to process initial screen"), strings.Contains(msg, "failed during initial handshake"):
		code = CodeNegotiation
	}
	return wrapError(code, fmt.Sprintf("failed to connect to %s:%d", host, port), err)
}

// Must be called before Connect — the model determines the screen buffer size
// allocated at handshake time.
func (c *Client) SetModel(model int) error {
	if model < 2 || model > 5 {
		return newError(CodeInvalidArgument, fmt.Sprintf("model must be between 2 and 5, got %d", model))
	}
	c.mu.Lock()
	c.model = model
	c.mu.Unlock()
	return nil
}

// Must be called before Connect. Supported: "cp037" (default), "cp1047".
func (c *Client) SetCodePage(name string) error {
	cp, ok := CodePages[name]
	if !ok {
		return newError(CodeInvalidArgument, fmt.Sprintf("unsupported code page: %s (supported: cp037, cp1047)", name))
	}
	c.mu.Lock()
	c.codePage = cp
	c.mu.Unlock()
	return nil
}

func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	if c.emu != nil {
		c.emu.Disconnect()
	}
	c.flushBytes()
	if !c.sessionStart.IsZero() {
		c.pushMetric(c.metrics.SessionDuration, float64(time.Since(c.sessionStart).Milliseconds()))
	}
	c.pushMetric(c.metrics.Disconnects, 1)

	c.connected = false
	c.emu = nil
	c.sessionStart = time.Time{}
	// Guarded close: a second Disconnect (e.g. from the janitor racing with an
	// explicit call) would panic on a re-closed nil channel.
	if c.janitorStop != nil {
		close(c.janitorStop)
		c.janitorStop = nil
	}
	return nil
}

func (c *Client) String(text string) error {
	if len(text) > 1920 {
		return newError(CodeInvalidArgument, "text exceeds maximum length of 1920 characters")
	}
	if err := c.checkConnected(); err != nil {
		return err
	}
	if err := c.emu.TypeString(text); err != nil {
		if strings.Contains(err.Error(), "protected field") {
			return wrapError(CodeProtectedField, "cannot type in protected field", err)
		}
		return wrapError(CodeProtocol, "type failed", err)
	}
	return nil
}

// Galasa's terminal API uses type(); String() is the s3270-style alias.
func (c *Client) Type(text string) error {
	return c.String(text)
}

func (c *Client) Enter() error {
	return c.sendAidOp(aidEnter, "Enter")
}

func (c *Client) Tab() error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	c.emu.Tab()
	return nil
}

func (c *Client) BackTab() error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	c.emu.BackTab()
	return nil
}

func (c *Client) Home() error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	c.emu.Home()
	return nil
}

func (c *Client) Clear() error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	ctx := c.vu.Context()
	start := time.Now()
	if err := c.emu.Clear(ctx, 30*time.Second); err != nil {
		c.flushBytes()
		c.pushMetric(c.metrics.Errors, 1)
		return wrapError(CodeProtocol, "clear failed", err)
	}
	c.pushMetric(c.metrics.SendDuration, float64(time.Since(start).Milliseconds()))
	c.pushMetric(c.metrics.AIDsSent, 1)
	c.pushMetric(c.metrics.Screens, 1)
	c.flushBytes()
	return nil
}

func (c *Client) sendAidOp(aid byte, label string) error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	ctx := c.vu.Context()
	start := time.Now()
	var err error
	switch {
	case aid == aidEnter:
		err = c.emu.Enter(ctx, 30*time.Second)
	case aid == aidClear:
		err = c.emu.Clear(ctx, 30*time.Second)
	}
	if err != nil {
		c.flushBytes()
		c.pushMetric(c.metrics.Errors, 1)
		return wrapError(CodeProtocol, fmt.Sprintf("%s failed", label), err)
	}
	c.pushMetric(c.metrics.SendDuration, float64(time.Since(start).Milliseconds()))
	c.pushMetric(c.metrics.AIDsSent, 1)
	c.pushMetric(c.metrics.Screens, 1)
	c.flushBytes()
	return nil
}

func (c *Client) Pf(key int) error {
	if key < 1 || key > 24 {
		return newError(CodeInvalidArgument, fmt.Sprintf("pf key must be between 1 and 24, got %d", key))
	}
	if err := c.checkConnected(); err != nil {
		return err
	}
	ctx := c.vu.Context()
	start := time.Now()
	if err := c.emu.PF(ctx, key, 30*time.Second); err != nil {
		c.flushBytes()
		c.pushMetric(c.metrics.Errors, 1)
		return wrapError(CodeProtocol, fmt.Sprintf("PF%d failed", key), err)
	}
	c.pushMetric(c.metrics.SendDuration, float64(time.Since(start).Milliseconds()))
	c.pushMetric(c.metrics.AIDsSent, 1)
	c.pushMetric(c.metrics.Screens, 1)
	c.flushBytes()
	return nil
}

func (c *Client) Pa(key int) error {
	if key < 1 || key > 3 {
		return newError(CodeInvalidArgument, fmt.Sprintf("pa key must be between 1 and 3, got %d", key))
	}
	if err := c.checkConnected(); err != nil {
		return err
	}
	ctx := c.vu.Context()
	start := time.Now()
	if err := c.emu.PA(ctx, key, 30*time.Second); err != nil {
		c.flushBytes()
		c.pushMetric(c.metrics.Errors, 1)
		return wrapError(CodeProtocol, fmt.Sprintf("PA%d failed", key), err)
	}
	c.pushMetric(c.metrics.SendDuration, float64(time.Since(start).Milliseconds()))
	c.pushMetric(c.metrics.AIDsSent, 1)
	c.pushMetric(c.metrics.Screens, 1)
	c.flushBytes()
	return nil
}

func (c *Client) MoveTo(row, col int) error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	if row < 1 || row > c.emu.rows {
		return newError(CodeInvalidArgument, fmt.Sprintf("row must be between 1 and %d, got %d", c.emu.rows, row))
	}
	if col < 1 || col > c.emu.cols {
		return newError(CodeInvalidArgument, fmt.Sprintf("column must be between 1 and %d, got %d", c.emu.cols, col))
	}
	c.emu.MoveCursor(row-1, col-1)
	return nil
}

func (c *Client) StringAt(text string, row, col int) error {
	if err := c.MoveTo(row, col); err != nil {
		return err
	}
	return c.String(text)
}

func (c *Client) WaitForField(timeout ...int) error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	timeoutSec := 30
	if len(timeout) > 0 && timeout[0] > 0 {
		timeoutSec = timeout[0]
	}
	ctx := c.vu.Context()
	start := time.Now()
	if err := c.emu.WaitForField(ctx, time.Duration(timeoutSec)*time.Second); err != nil {
		c.flushBytes()
		c.pushMetric(c.metrics.Errors, 1)
		if strings.Contains(err.Error(), "timeout") {
			c.pushMetric(c.metrics.WaitTimeouts, 1)
			return wrapError(CodeWaitTimeout, "timeout waiting for input field", err)
		}
		return wrapError(CodeProtocol, "waitForField failed", err)
	}
	c.pushMetric(c.metrics.WaitDuration, float64(time.Since(start).Milliseconds()))
	c.flushBytes()
	return nil
}

func (c *Client) WaitForText(text string, timeout ...int) error {
	_, err := c.WaitForTextAndReturn(text, timeout...)
	return err
}

func (c *Client) WaitForTextAndReturn(text string, timeout ...int) (string, error) {
	if err := c.checkConnected(); err != nil {
		return "", err
	}

	timeoutSec := 30
	if len(timeout) > 0 && timeout[0] > 0 {
		timeoutSec = timeout[0]
	}

	ctx := c.vu.Context()
	start := time.Now()
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			c.pushMetric(c.metrics.Errors, 1)
			return "", wrapError(CodeProtocol, "context cancelled", ctx.Err())
		default:
		}

		screen := c.emu.GetScreen()
		if strings.Contains(screen, text) {
			c.pushMetric(c.metrics.WaitDuration, float64(time.Since(start).Milliseconds()))
			return screen, nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	c.pushMetric(c.metrics.Errors, 1)
	c.pushMetric(c.metrics.WaitTimeouts, 1)
	return "", newError(CodeWaitTimeout, fmt.Sprintf("timeout waiting for text: %s", text))
}

func (c *Client) GetScreenText() (string, error) {
	if err := c.checkConnected(); err != nil {
		return "", err
	}
	return c.emu.GetScreen(), nil
}

func (c *Client) ASCII() (string, error) {
	return c.GetScreenText()
}

func (c *Client) SendCommand(command string, waitForResponse ...bool) error {
	if err := c.String(command); err != nil {
		return err
	}
	if err := c.Enter(); err != nil {
		return err
	}

	wait := true
	if len(waitForResponse) > 0 {
		wait = waitForResponse[0]
	}

	if wait {
		return c.WaitForField()
	}
	return nil
}

func (c *Client) SendPF(key int, waitForResponse ...bool) error {
	if err := c.Pf(key); err != nil {
		return err
	}

	wait := true
	if len(waitForResponse) > 0 {
		wait = waitForResponse[0]
	}

	if wait {
		return c.WaitForField()
	}
	return nil
}

func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

func (c *Client) Screenshot(path string) error {
	if path == "" {
		return newError(CodeInvalidArgument, "path cannot be empty")
	}

	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return newError(CodeInvalidArgument, "path cannot contain parent directory references")
	}

	screen, err := c.GetScreenText()
	if err != nil {
		return err
	}

	dir := filepath.Dir(cleanPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return wrapError(CodeScreenshotFailed, "failed to create directory", err)
		}
	}

	if err := os.WriteFile(cleanPath, []byte(screen), 0o600); err != nil {
		return wrapError(CodeScreenshotFailed, "failed to write screenshot", err)
	}

	return nil
}

func (c *Client) PrintScreen() (string, error) {
	screen, err := c.GetScreenText()
	if err != nil {
		return "", err
	}

	lines := strings.Split(screen, "\n")
	var result strings.Builder
	result.WriteString("┌" + strings.Repeat("─", 82) + "┐\n")
	for i, line := range lines {
		padded := line
		if len(padded) < 80 {
			padded += strings.Repeat(" ", 80-len(padded))
		}
		result.WriteString(fmt.Sprintf("│%2d│%s│\n", i+1, padded))
	}
	result.WriteString("└" + strings.Repeat("─", 82) + "┘")
	return result.String(), nil
}
