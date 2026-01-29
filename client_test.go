package tn3270

import (
	"strings"
	"testing"
)

func TestConnectValidation(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		port        int
		timeout     []int
		expectError string
	}{
		{
			name:        "empty host",
			host:        "",
			port:        23,
			expectError: "host cannot be empty",
		},
		{
			name:        "host too long",
			host:        string(make([]byte, 254)),
			port:        23,
			expectError: "host exceeds maximum length of 253 characters",
		},
		{
			name:        "port too low",
			host:        "localhost",
			port:        0,
			expectError: "port must be between 1 and 65535",
		},
		{
			name:        "port too high",
			host:        "localhost",
			port:        65536,
			expectError: "port must be between 1 and 65535",
		},
		{
			name:        "negative port",
			host:        "localhost",
			port:        -1,
			expectError: "port must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{}

			var err error
			if len(tt.timeout) > 0 {
				err = c.Connect(tt.host, tt.port, tt.timeout...)
			} else {
				err = c.Connect(tt.host, tt.port)
			}

			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestPfValidation(t *testing.T) {
	tests := []struct {
		name        string
		key         int
		expectError string
	}{
		{
			name:        "key too low",
			key:         0,
			expectError: "PF key must be between 1 and 24",
		},
		{
			name:        "key too high",
			key:         25,
			expectError: "PF key must be between 1 and 24",
		},
		{
			name:        "negative key",
			key:         -1,
			expectError: "PF key must be between 1 and 24",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{}
			err := c.Pf(tt.key)

			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestPaValidation(t *testing.T) {
	tests := []struct {
		name        string
		key         int
		expectError string
	}{
		{
			name:        "key too low",
			key:         0,
			expectError: "PA key must be between 1 and 3",
		},
		{
			name:        "key too high",
			key:         4,
			expectError: "PA key must be between 1 and 3",
		},
		{
			name:        "negative key",
			key:         -1,
			expectError: "PA key must be between 1 and 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{}
			err := c.Pa(tt.key)

			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestStringValidation(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		expectError string
	}{
		{
			name:        "text too long",
			text:        string(make([]byte, 1921)),
			expectError: "text exceeds maximum length of 1920 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{}
			err := c.String(tt.text)

			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestTypeValidation(t *testing.T) {
	c := &Client{}
	longText := string(make([]byte, 1921))

	err := c.Type(longText)
	if err == nil {
		t.Error("expected error for text too long, got nil")
		return
	}

	if !strings.Contains(err.Error(), "text exceeds maximum length of 1920 characters") {
		t.Errorf("expected length error, got %q", err.Error())
	}
}

func TestScreenshotValidation(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		expectError string
	}{
		{
			name:        "empty path",
			path:        "",
			expectError: "path cannot be empty",
		},
		{
			name:        "path traversal attempt",
			path:        "../../../etc/passwd",
			expectError: "path cannot contain parent directory references",
		},
		{
			name:        "path traversal in middle",
			path:        "screenshots/../../../etc/passwd",
			expectError: "path cannot contain parent directory references",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{}
			err := c.Screenshot(tt.path)

			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestIsConnected(t *testing.T) {
	c := &Client{connected: false}

	if c.IsConnected() {
		t.Error("expected IsConnected() to return false for new client")
	}

	c.connected = true
	if !c.IsConnected() {
		t.Error("expected IsConnected() to return true after setting connected=true")
	}
}

func TestDisconnectWhenNotConnected(t *testing.T) {
	c := &Client{connected: false}

	err := c.Disconnect()
	if err != nil {
		t.Errorf("expected nil error when disconnecting while not connected, got %v", err)
	}
}

func TestSendCommandWithoutS3270(t *testing.T) {
	c := &Client{}

	_, err := c.sendCommand("Test()")
	if err == nil {
		t.Error("expected error when calling sendCommand without starting s3270")
	}

	if !strings.Contains(err.Error(), "s3270 not started") {
		t.Errorf("expected error containing 's3270 not started', got %q", err.Error())
	}
}

func TestMoveToValidation(t *testing.T) {
	tests := []struct {
		name        string
		row         int
		col         int
		expectError string
	}{
		{
			name:        "row too low",
			row:         0,
			col:         1,
			expectError: "row must be between 1 and 24",
		},
		{
			name:        "row too high",
			row:         25,
			col:         1,
			expectError: "row must be between 1 and 24",
		},
		{
			name:        "col too low",
			row:         1,
			col:         0,
			expectError: "column must be between 1 and 80",
		},
		{
			name:        "col too high",
			row:         1,
			col:         81,
			expectError: "column must be between 1 and 80",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{}
			err := c.MoveTo(tt.row, tt.col)

			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}
