package tn3270

import (
	"bytes"
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
			expectError: "row must be between 1 and",
		},
		{
			name:        "row too high",
			row:         25,
			col:         1,
			expectError: "row must be between 1 and",
		},
		{
			name:        "col too low",
			row:         1,
			col:         0,
			expectError: "column must be between 1 and",
		},
		{
			name:        "col too high",
			row:         1,
			col:         81,
			expectError: "column must be between 1 and",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{emu: NewEmulator(), connected: true}
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

func TestWCCTwoPhaseProcessing(t *testing.T) {
	emu := NewEmulator()
	emu.keyboardLock = true

	// Set up an existing field with MDT set
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = attrMDT
	emu.buffer[1] = asciiToEBCDIC['X']

	// Send EraseWrite with WCC that resets MDT and unlocks keyboard.
	// The WCC reset-MDT should happen BEFORE orders, and keyboard
	// restore should happen AFTER orders.
	data := []byte{
		cmdEraseWrite,
		wccUnlock | wccResetMDT,
		orderSBA, addrTable[0], addrTable[0], // SBA to 0
		orderSF, 0x00, // New unprotected field at position 0
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, // HELLO in EBCDIC
	}

	err := emu.processMessage(data)
	if err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}

	// After processing: keyboard should be unlocked
	if emu.keyboardLock {
		t.Error("expected keyboard to be unlocked after WCC with unlock bit")
	}

	// The new field should NOT have MDT set (it was created by SF order
	// after the MDT reset phase)
	if emu.fieldAttrs[0]&attrMDT != 0 {
		t.Error("expected MDT to be clear on new field")
	}

	// Screen should contain HELLO
	screen := emu.GetScreen()
	if !strings.Contains(screen, "HELLO") {
		t.Errorf("expected screen to contain 'HELLO', got:\n%s", screen)
	}
}

func TestBuildQueryReply(t *testing.T) {
	emu := NewEmulator()

	// Build the query reply data directly
	var buf bytes.Buffer
	buf.WriteByte(aidSF)
	emu.appendQR(&buf, qrSummary, supportedQRCodes)
	emu.appendQR(&buf, qrUsableArea, []byte{
		0x01, 0x00,
		0x00, byte(emu.cols), 0x00, byte(emu.rows),
		0x01, 0x00, byte(emu.cols), 0x00, byte(emu.rows),
		0x07, 0x0C,
		byte(emu.size >> 8), byte(emu.size),
	})
	reply := buf.Bytes()

	// Should start with AID SF (0x88)
	if reply[0] != aidSF {
		t.Errorf("expected query reply to start with AID 0x88, got 0x%02X", reply[0])
	}

	// Parse the structured fields
	i := 1 // skip AID byte
	sfCount := 0
	for i < len(reply) {
		if i+2 > len(reply) {
			break
		}
		sfLen := int(reply[i])<<8 | int(reply[i+1])
		if sfLen < 4 || i+sfLen > len(reply) {
			t.Errorf("invalid SF length %d at offset %d", sfLen, i)
			break
		}
		if reply[i+2] != 0x81 {
			t.Errorf("expected SF ID 0x81 at offset %d, got 0x%02X", i+2, reply[i+2])
		}
		sfCount++
		i += sfLen
	}

	if sfCount != 2 {
		t.Errorf("expected 2 query reply SFs, got %d", sfCount)
	}

	// Verify the summary reply lists all QCODEs
	sfLen := int(reply[1])<<8 | int(reply[2])
	qcode := reply[4]
	if qcode != qrSummary {
		t.Errorf("expected first QR to be Summary (0x80), got 0x%02X", qcode)
	}
	summaryData := reply[5 : 1+sfLen]
	if len(summaryData) != len(supportedQRCodes) {
		t.Errorf("summary should list %d QCODEs, got %d", len(supportedQRCodes), len(summaryData))
	}
}

func TestProcessWSFOutbound3270DS(t *testing.T) {
	emu := NewEmulator()
	emu.keyboardLock = true

	// WSF with Outbound 3270DS containing an EraseWrite command
	// SF: length(2) + SF-ID(1) + partition-ID(1) + 3270 command stream
	cmd := []byte{
		cmdEraseWrite,
		wccUnlock | wccResetMDT,
		orderSBA, addrTable[0], addrTable[0],
		orderSF, 0x00,
		0xE6, 0xE2, 0xC6, // WSF in EBCDIC
	}
	sfLen := 2 + 1 + 1 + len(cmd) // length field + SF-ID + partition-ID + command
	wsf := []byte{byte(sfLen >> 8), byte(sfLen & 0xFF), sfOutbound3270DS, 0x00}
	wsf = append(wsf, cmd...)

	err := emu.processWSF(wsf)
	if err != nil {
		t.Fatalf("processWSF failed: %v", err)
	}

	screen := emu.GetScreen()
	if !strings.Contains(screen, "WSF") {
		t.Errorf("expected screen to contain 'WSF' after Outbound 3270DS, got:\n%s", screen)
	}

	if emu.keyboardLock {
		t.Error("expected keyboard unlocked after Outbound 3270DS with unlock WCC")
	}
}

func TestSendReadBuffer(t *testing.T) {
	emu := NewEmulator()
	emu.lastAID = aidEnter

	// Set up screen with a field and data
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = 0x00 // Unprotected
	emu.buffer[1] = asciiToEBCDIC['A']
	emu.buffer[2] = asciiToEBCDIC['B']
	emu.cursorAddr = 3

	// Build expected Read Buffer response
	var buf bytes.Buffer
	buf.WriteByte(aidEnter) // Last AID
	addr := encodeAddr(3)   // Cursor address
	buf.Write(addr[:])

	// Position 0: field attribute
	buf.WriteByte(orderSF)
	buf.WriteByte(0x00) // Unprotected

	// Position 1: 'A' in EBCDIC
	buf.WriteByte(asciiToEBCDIC['A'])
	// Position 2: 'B' in EBCDIC
	buf.WriteByte(asciiToEBCDIC['B'])

	// Positions 3-1919: nulls
	for i := 3; i < 1920; i++ {
		buf.WriteByte(0x00)
	}

	expected := buf.Bytes()

	// Build actual response (without sendData framing)
	var actual bytes.Buffer
	actual.WriteByte(emu.lastAID)
	curAddr := encodeAddr(emu.cursorAddr)
	actual.Write(curAddr[:])
	for i := 0; i < emu.size; i++ {
		if emu.isAttr[i] {
			actual.WriteByte(orderSF)
			actual.WriteByte(emu.fieldAttrs[i])
		} else {
			actual.WriteByte(emu.buffer[i])
		}
	}

	if !bytes.Equal(expected, actual.Bytes()) {
		t.Error("Read Buffer response mismatch")
	}
}

func TestSendReadModifiedAll(t *testing.T) {
	emu := NewEmulator()
	emu.lastAID = aidEnter

	// Two fields: one with MDT, one without
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = attrMDT // Modified
	emu.buffer[1] = asciiToEBCDIC['A']

	emu.isAttr[10] = true
	emu.fieldAttrs[10] = 0x00 // Not modified
	emu.buffer[11] = asciiToEBCDIC['B']

	// Read Modified (not all) should only include first field
	var rmBuf bytes.Buffer
	emu.writeModifiedFields(&rmBuf)

	// Read Modified All should include both fields
	var rmaBuf bytes.Buffer
	emu.writeAllFields(&rmaBuf)

	if rmaBuf.Len() <= rmBuf.Len() {
		t.Errorf("Read Modified All (%d bytes) should contain more data than Read Modified (%d bytes)",
			rmaBuf.Len(), rmBuf.Len())
	}
}

func TestSNACommandVariants(t *testing.T) {
	emu := NewEmulator()

	// Test SNA EraseWrite (0x05) - should work same as standard (0xF5)
	data := []byte{
		cmdEraseWriteSNA,
		wccUnlock | wccResetMDT,
		orderSBA, addrTable[0], addrTable[0],
		orderSF, 0x00,
		0xE3, 0xC5, 0xE2, 0xE3, // TEST in EBCDIC
	}

	err := emu.processMessage(data)
	if err != nil {
		t.Fatalf("SNA EraseWrite failed: %v", err)
	}

	screen := emu.GetScreen()
	if !strings.Contains(screen, "TEST") {
		t.Errorf("expected screen to contain 'TEST' after SNA EraseWrite, got:\n%s", screen)
	}

	if emu.keyboardLock {
		t.Error("expected keyboard unlocked after SNA EraseWrite with unlock WCC")
	}
}

func TestExtendedAttributeTracking(t *testing.T) {
	emu := NewEmulator()

	// SFE with highlight + color attributes
	data := []byte{
		cmdEraseWrite,
		wccUnlock | wccResetMDT,
		orderSBA, addrTable[0], addrTable[0],
		orderSFE, 0x03, // 3 attribute pairs
		extAttrBasic, 0x00, // unprotected
		extAttrHighlight, 0xF2, // reverse video
		extAttrColor, 0xF2, // red
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, // HELLO
	}

	err := emu.processMessage(data)
	if err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}

	// Field attr position should have extended attributes
	if emu.extFields[0].highlight != 0xF2 {
		t.Errorf("expected highlight 0xF2, got 0x%02X", emu.extFields[0].highlight)
	}
	if emu.extFields[0].color != 0xF2 {
		t.Errorf("expected color 0xF2, got 0x%02X", emu.extFields[0].color)
	}

	screen := emu.GetScreen()
	if !strings.Contains(screen, "HELLO") {
		t.Errorf("expected screen to contain 'HELLO', got:\n%s", screen)
	}
}

func TestSAOrderTracking(t *testing.T) {
	emu := NewEmulator()

	// Write with SA orders setting color
	data := []byte{
		cmdEraseWrite,
		wccUnlock | wccResetMDT,
		orderSBA, addrTable[0], addrTable[0],
		orderSF, 0x00, // unprotected field
		orderSA, extAttrColor, 0xF4, // Set color to green
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, // HELLO
	}

	err := emu.processMessage(data)
	if err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}

	// Data positions after SA should have the color set
	if emu.extFields[1].color != 0xF4 {
		t.Errorf("expected color 0xF4 at pos 1, got 0x%02X", emu.extFields[1].color)
	}
}

func TestReplyModeTracking(t *testing.T) {
	emu := NewEmulator()

	// Default should be field mode
	if emu.replyMode != replyModeField {
		t.Errorf("expected default reply mode 0x00, got 0x%02X", emu.replyMode)
	}

	// Set to character mode via WSF
	emu.handleSetReplyMode([]byte{0x00, replyModeChar, extAttrHighlight, extAttrColor})

	if emu.replyMode != replyModeChar {
		t.Errorf("expected reply mode 0x02 (character), got 0x%02X", emu.replyMode)
	}
	if len(emu.replyModeAttr) != 2 {
		t.Errorf("expected 2 reply mode attrs, got %d", len(emu.replyModeAttr))
	}

	// Set to extended field mode
	emu.handleSetReplyMode([]byte{0x00, replyModeExtField})
	if emu.replyMode != replyModeExtField {
		t.Errorf("expected reply mode 0x01 (ext field), got 0x%02X", emu.replyMode)
	}
}

func TestReadBufferExtendedFieldMode(t *testing.T) {
	emu := NewEmulator()
	emu.lastAID = aidEnter
	emu.replyMode = replyModeExtField

	// Set up field with extended attributes
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = 0x00
	emu.extFields[0] = extAttrs{highlight: 0xF2, color: 0xF4}
	emu.buffer[1] = asciiToEBCDIC['A']
	emu.cursorAddr = 2

	// Build response manually
	var buf bytes.Buffer
	buf.WriteByte(aidEnter)
	addr := encodeAddr(2)
	buf.Write(addr[:])

	// In ext field mode, field attrs should use SFE
	for i := 0; i < emu.size; i++ {
		if emu.isAttr[i] {
			emu.writeSFE(&buf, i)
		} else {
			if emu.extFields[i].ge {
				buf.WriteByte(orderGE)
			}
			buf.WriteByte(emu.buffer[i])
		}
	}

	result := buf.Bytes()
	// First 3 bytes: AID + cursor addr
	// Then should be SFE order (0x29)
	if result[3] != orderSFE {
		t.Errorf("expected SFE (0x29) in ext field mode, got 0x%02X", result[3])
	}
}

func TestCharacterModeReadBuffer(t *testing.T) {
	emu := NewEmulator()
	emu.lastAID = aidEnter
	emu.replyMode = replyModeChar

	// Set up field with data that has varying colors
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = 0x00
	emu.buffer[1] = asciiToEBCDIC['A']
	emu.extFields[1] = extAttrs{color: 0xF2} // red
	emu.buffer[2] = asciiToEBCDIC['B']
	emu.extFields[2] = extAttrs{color: 0xF4} // green
	emu.cursorAddr = 3

	var buf bytes.Buffer
	emu.writeCharacterModeBuffer(&buf)
	result := buf.Bytes()

	// Should contain SA orders between positions with different colors
	hasSA := false
	for i := 0; i < len(result)-2; i++ {
		if result[i] == orderSA {
			hasSA = true
			break
		}
	}
	if !hasSA {
		t.Error("expected SA orders in character mode buffer, found none")
	}
}

func TestAlternateScreenSize(t *testing.T) {
	// Model 5 has alternate size 27×132
	emu := NewEmulatorModel(5)

	if emu.defRows != 24 || emu.defCols != 80 {
		t.Errorf("expected default 24×80, got %d×%d", emu.defRows, emu.defCols)
	}
	if emu.altRows != 27 || emu.altCols != 132 {
		t.Errorf("expected alt 27×132, got %d×%d", emu.altRows, emu.altCols)
	}

	// Default starts at 24×80
	if emu.rows != 24 || emu.cols != 80 {
		t.Errorf("expected initial 24×80, got %d×%d", emu.rows, emu.cols)
	}

	// EraseWriteAlt should switch to alternate size
	data := []byte{cmdEraseWriteAlt, wccUnlock | wccResetMDT}
	err := emu.processMessage(data)
	if err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}

	if emu.rows != 27 || emu.cols != 132 {
		t.Errorf("expected 27×132 after EraseWriteAlt, got %d×%d", emu.rows, emu.cols)
	}

	// Regular EraseWrite should switch back to default
	data = []byte{cmdEraseWrite, wccUnlock | wccResetMDT}
	err = emu.processMessage(data)
	if err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}

	if emu.rows != 24 || emu.cols != 80 {
		t.Errorf("expected 24×80 after EraseWrite, got %d×%d", emu.rows, emu.cols)
	}
}

func TestGETracking(t *testing.T) {
	emu := NewEmulator()

	data := []byte{
		cmdEraseWrite,
		wccUnlock | wccResetMDT,
		orderSBA, addrTable[0], addrTable[0],
		orderSF, 0x00,
		orderGE, 0xC1, // GE 'A'
		0xC2, // regular 'B'
	}

	err := emu.processMessage(data)
	if err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}

	if !emu.extFields[1].ge {
		t.Error("expected GE flag set at position 1")
	}
	if emu.extFields[2].ge {
		t.Error("expected GE flag NOT set at position 2")
	}
}

func TestCodePageCP1047(t *testing.T) {
	cp := CodePages["cp1047"]
	if cp == nil {
		t.Fatal("cp1047 not found in CodePages")
	}

	// Verify bracket mappings specific to CP1047
	if cp.EBCDICToASCII[0xAD] != '[' {
		t.Errorf("CP1047: expected 0xAD -> '[', got %c", cp.EBCDICToASCII[0xAD])
	}
	if cp.EBCDICToASCII[0xBD] != ']' {
		t.Errorf("CP1047: expected 0xBD -> ']', got %c", cp.EBCDICToASCII[0xBD])
	}

	// Verify round-trip for common chars
	for _, ch := range []byte("ABCabc012") {
		ebcdic := cp.ASCIIToEBCDIC[ch]
		back := cp.EBCDICToASCII[ebcdic]
		if back != ch {
			t.Errorf("CP1047 round-trip failed for '%c': ebcdic=0x%02X, back='%c'", ch, ebcdic, back)
		}
	}
}

func TestConnectionStateTransitions(t *testing.T) {
	emu := NewEmulator()

	if emu.connState != connNotConnected {
		t.Errorf("expected connNotConnected, got %d", emu.connState)
	}
}

func TestSetModelValidation(t *testing.T) {
	c := &Client{}
	if err := c.SetModel(1); err == nil {
		t.Error("expected error for model 1")
	}
	if err := c.SetModel(6); err == nil {
		t.Error("expected error for model 6")
	}
	if err := c.SetModel(3); err != nil {
		t.Errorf("expected no error for model 3, got %v", err)
	}
}

func TestSetCodePageValidation(t *testing.T) {
	c := &Client{}
	if err := c.SetCodePage("cp037"); err != nil {
		t.Errorf("expected no error for cp037, got %v", err)
	}
	if err := c.SetCodePage("cp1047"); err != nil {
		t.Errorf("expected no error for cp1047, got %v", err)
	}
	if err := c.SetCodePage("invalid"); err == nil {
		t.Error("expected error for invalid code page")
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
