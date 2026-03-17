package tn3270

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.k6.io/k6/js/modules"
)

type Client struct {
	vu        modules.VU
	emu       *Emulator
	mu        sync.Mutex
	connected bool
}

func NewClient(vu modules.VU) *Client {
	return &Client{
		vu: vu,
	}
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
	c.emu = NewEmulator()
	c.mu.Unlock()

	if err := c.emu.Connect(host, port, time.Duration(timeoutSec)*time.Second); err != nil {
		c.mu.Lock()
		c.emu = nil
		c.mu.Unlock()
		return fmt.Errorf("failed to connect to %s:%d: %w", host, port, err)
	}

	c.mu.Lock()
	c.connected = true
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
	return c.emu.Enter(30 * time.Second)
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
	return c.emu.Clear(30 * time.Second)
}

func (c *Client) Pf(key int) error {
	if key < 1 || key > 24 {
		return fmt.Errorf("pf key must be between 1 and 24, got %d", key)
	}
	if err := c.checkConnected(); err != nil {
		return err
	}
	return c.emu.PF(key, 30*time.Second)
}

func (c *Client) Pa(key int) error {
	if key < 1 || key > 3 {
		return fmt.Errorf("pa key must be between 1 and 3, got %d", key)
	}
	if err := c.checkConnected(); err != nil {
		return err
	}
	return c.emu.PA(key, 30*time.Second)
}

func (c *Client) MoveTo(row, col int) error {
	if row < 1 || row > 24 {
		return fmt.Errorf("row must be between 1 and 24, got %d", row)
	}
	if col < 1 || col > 80 {
		return fmt.Errorf("column must be between 1 and 80, got %d", col)
	}
	if err := c.checkConnected(); err != nil {
		return err
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
	return c.emu.WaitForField(time.Duration(timeoutSec) * time.Second)
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
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		screen := c.emu.GetScreen()
		if strings.Contains(screen, text) {
			return screen, nil
		}

		time.Sleep(100 * time.Millisecond)
	}

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
