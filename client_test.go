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
			expectError: "pf key must be between 1 and 24",
		},
		{
			name:        "key too high",
			key:         25,
			expectError: "pf key must be between 1 and 24",
		},
		{
			name:        "negative key",
			key:         -1,
			expectError: "pf key must be between 1 and 24",
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
			expectError: "pa key must be between 1 and 3",
		},
		{
			name:        "key too high",
			key:         4,
			expectError: "pa key must be between 1 and 3",
		},
		{
			name:        "negative key",
			key:         -1,
			expectError: "pa key must be between 1 and 3",
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

func TestOperationWithoutConnection(t *testing.T) {
	c := &Client{}

	err := c.Enter()
	if err == nil {
		t.Fatal("expected error when calling Enter without connection")
	}

	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("expected error containing 'not connected', got %q", err.Error())
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

func TestEBCDICRoundTrip(t *testing.T) {
	// Test that common ASCII characters round-trip correctly
	testChars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789 .,:;!@#$%^&*()-_=+/<>?'\""
	for _, ch := range []byte(testChars) {
		ebcdic := asciiToEBCDIC[ch]
		if ebcdic == 0x00 && ch != 0x00 {
			t.Errorf("ASCII 0x%02X (%c) maps to EBCDIC 0x00", ch, ch)
			continue
		}
		back := ebcdicToASCII[ebcdic]
		if back != ch {
			t.Errorf("round-trip failed for ASCII 0x%02X (%c): EBCDIC=0x%02X, back=0x%02X (%c)", ch, ch, ebcdic, back, back)
		}
	}
}

func TestBufferAddressRoundTrip(t *testing.T) {
	// Test buffer address encoding/decoding for all valid positions
	for addr := 0; addr < 1920; addr++ {
		encoded := encodeAddr(addr)
		decoded := decodeAddr(encoded[0], encoded[1])
		if decoded != addr {
			t.Errorf("address round-trip failed for %d: encoded=[0x%02X,0x%02X], decoded=%d",
				addr, encoded[0], encoded[1], decoded)
		}
	}
}

func TestEmulatorScreenBuffer(t *testing.T) {
	emu := NewEmulator()

	// Simulate a simple EraseWrite with a field
	// Command: EraseWrite, WCC (unlock+reset MDT), SF(unprotected), "HELLO"
	data := []byte{
		cmdEraseWrite,
		wccUnlock | wccResetMDT,
		orderSBA, addrTable[0], addrTable[0], // SBA to position 0
		orderSF, 0x00, // Start unprotected field
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, // HELLO in EBCDIC
	}

	err := emu.processMessage(data)
	if err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}

	screen := emu.GetScreen()
	if !strings.Contains(screen, "HELLO") {
		t.Errorf("expected screen to contain 'HELLO', got:\n%s", screen)
	}

	if emu.keyboardLock {
		t.Error("expected keyboard to be unlocked after WCC with unlock bit")
	}
}

func TestEmulatorTypeString(t *testing.T) {
	emu := NewEmulator()

	// Set up a simple unprotected field
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = 0x00 // Unprotected
	emu.cursorAddr = 1       // First data position
	emu.keyboardLock = false

	err := emu.TypeString("TEST")
	if err != nil {
		t.Fatalf("TypeString failed: %v", err)
	}

	// Verify EBCDIC data in buffer
	expected := []byte{asciiToEBCDIC['T'], asciiToEBCDIC['E'], asciiToEBCDIC['S'], asciiToEBCDIC['T']}
	for i, exp := range expected {
		if emu.buffer[i+1] != exp {
			t.Errorf("buffer[%d] = 0x%02X, expected 0x%02X", i+1, emu.buffer[i+1], exp)
		}
	}

	// Verify MDT was set
	if emu.fieldAttrs[0]&attrMDT == 0 {
		t.Error("expected MDT to be set on the field")
	}
}

func TestEmulatorHasUnlockedField(t *testing.T) {
	emu := NewEmulator()

	// No fields, keyboard locked
	emu.keyboardLock = true
	if emu.HasUnlockedField() {
		t.Error("expected false when keyboard is locked")
	}

	// No fields, keyboard unlocked (unformatted screen)
	emu.keyboardLock = false
	if !emu.HasUnlockedField() {
		t.Error("expected true for unformatted screen with unlocked keyboard")
	}

	// Protected field only, keyboard unlocked
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = attrProtected
	if emu.HasUnlockedField() {
		t.Error("expected false when only protected fields exist")
	}

	// Add unprotected field
	emu.isAttr[80] = true
	emu.fieldAttrs[80] = 0x00 // Unprotected
	if !emu.HasUnlockedField() {
		t.Error("expected true when unprotected field exists")
	}
}
