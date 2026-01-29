package tn3270

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.k6.io/k6/js/modules"
)

type Client struct {
	vu        modules.VU
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	mu        sync.Mutex
	connected bool
}

func NewClient(vu modules.VU) *Client {
	return &Client{
		vu: vu,
	}
}

func (c *Client) startS3270() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil {
		return nil
	}

	// Don't use CommandContext - we manage the lifecycle manually via Disconnect()
	// Using VU context causes the process to be killed between iterations
	c.cmd = exec.Command("s3270")

	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	c.stdin = stdin

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	c.stdout = bufio.NewReader(stdout)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start s3270: %w", err)
	}

	return nil
}

func (c *Client) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
	c.cmd = nil
	c.connected = false
}

func (c *Client) sendCommand(command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd == nil {
		return "", fmt.Errorf("s3270 not started")
	}

	_, err := fmt.Fprintf(c.stdin, "%s\n", command)
	if err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	var result strings.Builder
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read response: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "ok" {
			break
		}
		if line == "error" || strings.HasPrefix(line, "error ") {
			errMsg := strings.TrimPrefix(line, "error ")
			if errMsg == line {
				errMsg = "unknown error"
			}
			return "", fmt.Errorf("s3270 error: %s (command: %s)", errMsg, command)
		}
		if strings.HasPrefix(line, "data:") {
			content := strings.TrimPrefix(line, "data:")
			content = strings.TrimPrefix(content, " ")
			result.WriteString(content)
			result.WriteString("\n")
		}
	}

	return strings.TrimSuffix(result.String(), "\n"), nil
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

	if err := c.startS3270(); err != nil {
		return err
	}

	_, err := c.sendCommand(fmt.Sprintf("Connect(%s:%d)", host, port))
	if err != nil {
		return fmt.Errorf("failed to connect to %s:%d: %w", host, port, err)
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	return nil
}

func (c *Client) Disconnect() error {
	c.mu.Lock()
	connected := c.connected
	c.mu.Unlock()

	if !connected {
		return nil
	}

	_, _ = c.sendCommand("Disconnect()")
	_, _ = c.sendCommand("Quit()")
	c.cleanup()

	return nil
}

func (c *Client) String(text string) error {
	if len(text) > 1920 {
		return fmt.Errorf("text exceeds maximum length of 1920 characters")
	}
	_, err := c.sendCommand(fmt.Sprintf("String(%q)", text))
	return err
}

// Type is an alias for String, matching Galasa's API convention.
func (c *Client) Type(text string) error {
	return c.String(text)
}

func (c *Client) Enter() error {
	_, err := c.sendCommand("Enter()")
	return err
}

func (c *Client) Tab() error {
	_, err := c.sendCommand("Tab()")
	return err
}

func (c *Client) BackTab() error {
	_, err := c.sendCommand("BackTab()")
	return err
}

func (c *Client) Home() error {
	_, err := c.sendCommand("Home()")
	return err
}

func (c *Client) Clear() error {
	_, err := c.sendCommand("Clear()")
	return err
}

func (c *Client) Pf(key int) error {
	if key < 1 || key > 24 {
		return fmt.Errorf("PF key must be between 1 and 24, got %d", key)
	}
	_, err := c.sendCommand(fmt.Sprintf("PF(%d)", key))
	return err
}

func (c *Client) Pa(key int) error {
	if key < 1 || key > 3 {
		return fmt.Errorf("PA key must be between 1 and 3, got %d", key)
	}
	_, err := c.sendCommand(fmt.Sprintf("PA(%d)", key))
	return err
}

func (c *Client) MoveTo(row, col int) error {
	if row < 1 || row > 24 {
		return fmt.Errorf("row must be between 1 and 24, got %d", row)
	}
	if col < 1 || col > 80 {
		return fmt.Errorf("column must be between 1 and 80, got %d", col)
	}
	_, err := c.sendCommand(fmt.Sprintf("MoveCursor(%d,%d)", row-1, col-1))
	return err
}

func (c *Client) StringAt(text string, row, col int) error {
	if err := c.MoveTo(row, col); err != nil {
		return err
	}
	return c.String(text)
}

func (c *Client) WaitForField(timeout ...int) error {
	timeoutSec := 30
	if len(timeout) > 0 && timeout[0] > 0 {
		timeoutSec = timeout[0]
	}
	_, err := c.sendCommand(fmt.Sprintf("Wait(%d,InputField)", timeoutSec))
	return err
}

func (c *Client) WaitForText(text string, timeout ...int) error {
	_, err := c.WaitForTextAndReturn(text, timeout...)
	return err
}

func (c *Client) WaitForTextAndReturn(text string, timeout ...int) (string, error) {
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

		screen, err := c.GetScreenText()
		if err != nil {
			return "", err
		}

		if strings.Contains(screen, text) {
			return screen, nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return "", fmt.Errorf("timeout waiting for text: %s", text)
}

func (c *Client) GetScreenText() (string, error) {
	return c.sendCommand("Ascii()")
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
		if err := os.MkdirAll(dir, 0o755); err != nil {
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
