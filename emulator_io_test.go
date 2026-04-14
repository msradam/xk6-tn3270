package tn3270

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"
)

func TestEmulatorEnterSendsAIDAndReceivesScreen(t *testing.T) {
	client, server := net.Pipe()
	_ = server.SetDeadline(time.Now().Add(5 * time.Second))
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))

	emu := NewEmulator()
	emu.conn = client
	emu.reader = bufio.NewReader(client)
	// Pre-seed an unprotected field so HasUnlockedField passes on the return
	// screen after the host echo.
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = 0x00
	emu.keyboardLock = false

	hostDone := make(chan error, 1)
	go func() {
		// Drain the Enter AID record the emulator sends.
		hostReader := bufio.NewReader(server)
		for {
			b, err := hostReader.ReadByte()
			if err != nil {
				hostDone <- err
				return
			}
			if b == telnetIAC {
				nxt, err := hostReader.ReadByte()
				if err != nil {
					hostDone <- err
					return
				}
				if nxt == telnetEOR {
					break
				}
			}
		}
		// Reply: EraseWrite "OK" with unlock WCC.
		reply := []byte{
			cmdEraseWrite, wccUnlock | wccResetMDT,
			orderSBA, addrTable[0], addrTable[0],
			orderSF, 0x00,
			0xD6, 0xD2, // OK in EBCDIC
			telnetIAC, telnetEOR,
		}
		_, err := server.Write(reply)
		hostDone <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := emu.Enter(ctx, 3*time.Second); err != nil {
		t.Fatalf("Enter: %v", err)
	}
	if err := <-hostDone; err != nil {
		t.Fatalf("host goroutine: %v", err)
	}
	if emu.keyboardLock {
		t.Error("keyboard should unlock after host reply")
	}
	if emu.BytesOut() == 0 {
		t.Error("BytesOut should have incremented on Enter")
	}
	if emu.BytesIn() == 0 {
		t.Error("BytesIn should have incremented from host reply")
	}
}

func TestEmulatorTabMovesThroughFields(t *testing.T) {
	emu := NewEmulator()
	// Two unprotected fields, one protected in between.
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = 0x00 // unprotected, data starts at 1
	emu.isAttr[10] = true
	emu.fieldAttrs[10] = attrProtected
	emu.isAttr[20] = true
	emu.fieldAttrs[20] = 0x00

	emu.cursorAddr = 5
	emu.Tab()
	// Expect cursor to jump past the protected field to pos 21.
	if emu.cursorAddr != 21 {
		t.Errorf("Tab cursor = %d, want 21", emu.cursorAddr)
	}
	// BackTab from the middle of the second field should jump to field start.
	emu.cursorAddr = 25
	emu.BackTab()
	if emu.cursorAddr != 21 {
		t.Errorf("BackTab cursor = %d, want 21", emu.cursorAddr)
	}
}

func TestEmulatorHomeOnUnformatted(t *testing.T) {
	emu := NewEmulator()
	emu.cursorAddr = 100
	emu.Home()
	if emu.cursorAddr != 0 {
		t.Errorf("Home on unformatted cursor = %d, want 0", emu.cursorAddr)
	}
}

func TestEmulatorMoveCursorBounds(t *testing.T) {
	emu := NewEmulator()
	emu.MoveCursor(0, 0)
	if emu.cursorAddr != 0 {
		t.Errorf("MoveCursor(0,0) = %d", emu.cursorAddr)
	}
	emu.MoveCursor(1, 5)
	if emu.cursorAddr != 85 {
		t.Errorf("MoveCursor(1,5) = %d, want 85", emu.cursorAddr)
	}
}

func TestEmulatorDisconnectIdempotent(t *testing.T) {
	emu := NewEmulator()
	// Disconnect without a connection should be a no-op, not a panic.
	emu.Disconnect()
	emu.Disconnect()
	if emu.IsConnected() {
		t.Error("IsConnected() should be false after Disconnect")
	}
}

func TestEmulatorDisconnectWipesSecrets(t *testing.T) {
	emu := NewEmulator()
	// Simulate a session that typed a password and then disconnected without
	// sending Enter — the buffer would normally retain the EBCDIC plaintext
	// until GC. wipeBuffers should zero it.
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = attrNonDisplay
	password := []byte{0xD7, 0xC1, 0xE2, 0xE2} // "PASS" in EBCDIC
	copy(emu.buffer[1:], password)
	emu.cursorAddr = 5
	emu.tn3270e = true
	emu.seqNum = 42

	emu.Disconnect()

	for i, b := range emu.buffer {
		if b != 0 {
			t.Errorf("buffer[%d] = 0x%02X after Disconnect, want 0", i, b)
			break
		}
	}
	if emu.cursorAddr != 0 || emu.tn3270e || emu.seqNum != 0 || emu.lastAID != 0 {
		t.Errorf("session state not cleared: cursor=%d tn3270e=%v seq=%d aid=0x%02X",
			emu.cursorAddr, emu.tn3270e, emu.seqNum, emu.lastAID)
	}
}

func TestClassifyConnectError(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want string
	}{
		{"tcp-timeout", errorWithMsg("dial tcp: i/o timeout"), CodeConnectTimeout},
		{"handshake-timeout", errorWithMsg("timeout during initial handshake"), CodeConnectTimeout},
		{"tls-cert", errorWithMsg("x509: certificate signed by unknown authority"), CodeTLSHandshake},
		{"tls-handshake", errorWithMsg("tls: handshake failure"), CodeTLSHandshake},
		{"negotiation", errorWithMsg("failed to process initial screen: bad data"), CodeNegotiation},
		{"fallback", errorWithMsg("something else"), CodeConnectFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := asCode(classifyConnectError("h", 23, tc.in))
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

type stringErr string

func (s stringErr) Error() string { return string(s) }
func errorWithMsg(m string) error  { return stringErr(m) }
