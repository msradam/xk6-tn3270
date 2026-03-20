package tn3270

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/metrics"
)

// logrusAdapter wraps the k6 VU logger to satisfy the TraceLogger interface.
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
	vu        modules.VU
	emu       *Emulator
	mu        sync.Mutex
	connected bool
	model     int
	codePage  *CodePage
	trace     bool
	metrics   *tn3270Metrics
}

func NewClient(vu modules.VU, m *tn3270Metrics) *Client {
	return &Client{
		vu:      vu,
		metrics: m,
	}
}

// newEmulator creates an Emulator with the client's configured model and code page.
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

// pushMetric records a metric sample on the VU's state.
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

// SetTrace enables protocol-level debug tracing to stderr.
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
		return fmt.Errorf("not connected")
	}
	return nil
}

func (c *Client) Connect(host string, port int, timeout ...int) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}
	if len(host) > 253 {
		return fmt.Errorf("host exceeds maximum length of 253 characters")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}

	timeoutSec := 30
	if len(timeout) > 0 && timeout[0] > 0 {
		timeoutSec = timeout[0]
	}
	if timeoutSec < 1 || timeoutSec > 300 {
		return fmt.Errorf("timeout must be between 1 and 300 seconds, got %d", timeoutSec)
	}

	c.mu.Lock()
	c.emu = c.newEmulator()
	c.mu.Unlock()

	ctx := c.vu.Context()
	start := time.Now()

	if err := c.emu.Connect(ctx, host, port, time.Duration(timeoutSec)*time.Second); err != nil {
		c.pushMetric(c.metrics.Errors, 1)
		c.mu.Lock()
		c.emu = nil
		c.mu.Unlock()
		return fmt.Errorf("failed to connect to %s:%d: %w", host, port, err)
	}

	c.pushMetric(c.metrics.ConnectDuration, float64(time.Since(start).Milliseconds()))

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	return nil
}

// ConnectTLS establishes a TLS-encrypted TN3270 connection.
func (c *Client) ConnectTLS(host string, port int, insecure bool, timeout ...int) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}
	if len(host) > 253 {
		return fmt.Errorf("host exceeds maximum length of 253 characters")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}

	timeoutSec := 30
	if len(timeout) > 0 && timeout[0] > 0 {
		timeoutSec = timeout[0]
	}
	if timeoutSec < 1 || timeoutSec > 300 {
		return fmt.Errorf("timeout must be between 1 and 300 seconds, got %d", timeoutSec)
	}

	c.mu.Lock()
	c.emu = c.newEmulator()
	c.emu.useTLS = true
	c.emu.tlsInsecure = insecure
	c.mu.Unlock()

	ctx := c.vu.Context()
	start := time.Now()

	if err := c.emu.Connect(ctx, host, port, time.Duration(timeoutSec)*time.Second); err != nil {
		c.pushMetric(c.metrics.Errors, 1)
		c.mu.Lock()
		c.emu = nil
		c.mu.Unlock()
		return fmt.Errorf("failed to connect to %s:%d: %w", host, port, err)
	}

	c.pushMetric(c.metrics.ConnectDuration, float64(time.Since(start).Milliseconds()))

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	return nil
}

// SetModel sets the terminal model (2-5). Must be called before Connect.
func (c *Client) SetModel(model int) error {
	if model < 2 || model > 5 {
		return fmt.Errorf("model must be between 2 and 5, got %d", model)
	}
	c.mu.Lock()
	c.model = model
	c.mu.Unlock()
	return nil
}

// SetCodePage sets the EBCDIC code page. Supported: "cp037" (default), "cp1047".
// Must be called before Connect.
func (c *Client) SetCodePage(name string) error {
	cp, ok := CodePages[name]
	if !ok {
		return fmt.Errorf("unsupported code page: %s (supported: cp037, cp1047)", name)
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

	c.connected = false
	c.emu = nil
	return nil
}

func (c *Client) String(text string) error {
	if len(text) > 1920 {
		return fmt.Errorf("text exceeds maximum length of 1920 characters")
	}
	if err := c.checkConnected(); err != nil {
		return err
	}
	return c.emu.TypeString(text)
}

// Type is an alias for String, matching Galasa's API convention.
func (c *Client) Type(text string) error {
	return c.String(text)
}

func (c *Client) Enter() error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	ctx := c.vu.Context()
	start := time.Now()
	if err := c.emu.Enter(ctx, 30*time.Second); err != nil {
		c.pushMetric(c.metrics.Errors, 1)
		return err
	}
	c.pushMetric(c.metrics.SendDuration, float64(time.Since(start).Milliseconds()))
	c.pushMetric(c.metrics.Screens, 1)
	return nil
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
		c.pushMetric(c.metrics.Errors, 1)
		return err
	}
	c.pushMetric(c.metrics.SendDuration, float64(time.Since(start).Milliseconds()))
	c.pushMetric(c.metrics.Screens, 1)
	return nil
}

func (c *Client) Pf(key int) error {
	if key < 1 || key > 24 {
		return fmt.Errorf("pf key must be between 1 and 24, got %d", key)
	}
	if err := c.checkConnected(); err != nil {
		return err
	}
	ctx := c.vu.Context()
	start := time.Now()
	if err := c.emu.PF(ctx, key, 30*time.Second); err != nil {
		c.pushMetric(c.metrics.Errors, 1)
		return err
	}
	c.pushMetric(c.metrics.SendDuration, float64(time.Since(start).Milliseconds()))
	c.pushMetric(c.metrics.Screens, 1)
	return nil
}

func (c *Client) Pa(key int) error {
	if key < 1 || key > 3 {
		return fmt.Errorf("pa key must be between 1 and 3, got %d", key)
	}
	if err := c.checkConnected(); err != nil {
		return err
	}
	ctx := c.vu.Context()
	start := time.Now()
	if err := c.emu.PA(ctx, key, 30*time.Second); err != nil {
		c.pushMetric(c.metrics.Errors, 1)
		return err
	}
	c.pushMetric(c.metrics.SendDuration, float64(time.Since(start).Milliseconds()))
	c.pushMetric(c.metrics.Screens, 1)
	return nil
}

func (c *Client) MoveTo(row, col int) error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	if row < 1 || row > c.emu.rows {
		return fmt.Errorf("row must be between 1 and %d, got %d", c.emu.rows, row)
	}
	if col < 1 || col > c.emu.cols {
		return fmt.Errorf("column must be between 1 and %d, got %d", c.emu.cols, col)
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
		c.pushMetric(c.metrics.Errors, 1)
		return err
	}
	c.pushMetric(c.metrics.WaitDuration, float64(time.Since(start).Milliseconds()))
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
			return "", ctx.Err()
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
	return "", fmt.Errorf("timeout waiting for text: %s", text)
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
		return fmt.Errorf("path cannot be empty")
	}

	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path cannot contain parent directory references")
	}

	screen, err := c.GetScreenText()
	if err != nil {
		return fmt.Errorf("failed to get screen: %w", err)
	}

	dir := filepath.Dir(cleanPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	if err := os.WriteFile(cleanPath, []byte(screen), 0o600); err != nil {
		return fmt.Errorf("failed to write screenshot: %w", err)
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
