package tn3270

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// net.Pipe is synchronous, so every expect must pair with an emulator write
// or the test deadlocks — that's how we keep these exchanges deterministic.
type fakeHost struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
}

func newFakeHost(t *testing.T) (*fakeHost, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	// Below go test's 10m default so a misbehaving emulator fails fast.
	_ = server.SetDeadline(time.Now().Add(5 * time.Second))
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	return &fakeHost{t: t, conn: server, r: bufio.NewReader(server)}, client
}

func (h *fakeHost) write(b []byte) {
	h.t.Helper()
	if _, err := h.conn.Write(b); err != nil {
		h.t.Fatalf("fakeHost write: %v", err)
	}
}

func (h *fakeHost) readN(n int) []byte {
	h.t.Helper()
	buf := make([]byte, n)
	if _, err := io.ReadFull(h.r, buf); err != nil {
		h.t.Fatalf("fakeHost read %d: %v", n, err)
	}
	return buf
}

func (h *fakeHost) drainEOR() []byte {
	h.t.Helper()
	var out []byte
	for {
		b, err := h.r.ReadByte()
		if err != nil {
			h.t.Fatalf("fakeHost drainEOR: %v", err)
		}
		if b == telnetIAC {
			nxt, err := h.r.ReadByte()
			if err != nil {
				h.t.Fatalf("fakeHost drainEOR peek: %v", err)
			}
			if nxt == telnetEOR {
				return out
			}
			if nxt == telnetIAC {
				out = append(out, telnetIAC)
				continue
			}
			// Other IAC command (SB, DO, WILL, etc.) — put both back logically
			// by appending; test-specific scripts avoid this path.
			out = append(out, telnetIAC, nxt)
			continue
		}
		out = append(out, b)
	}
}

func connectFakeHost(t *testing.T, runHost func(*fakeHost)) *Emulator {
	t.Helper()
	host, client := newFakeHost(t)
	emu := NewEmulator()
	emu.conn = client
	emu.reader = bufio.NewReader(client)
	emu.keyboardLock = true
	emu.connState = connPending
	emu.eraseScreen()

	done := make(chan struct{})
	go func() {
		runHost(host)
		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := emu.handshake(ctx, 3*time.Second); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	<-done
	return emu
}

func TestEmulatorHandshakeAndScreen(t *testing.T) {
	emu := connectFakeHost(t, func(h *fakeHost) {
		// Server: IAC DO EOR → expect WILL EOR
		h.write([]byte{telnetIAC, telnetDO, telnetOptEOR})
		got := h.readN(3)
		if got[0] != telnetIAC || got[1] != telnetWILL || got[2] != telnetOptEOR {
			h.t.Fatalf("expected WILL EOR, got %x", got)
		}
		// Server: IAC DO BINARY → expect WILL BINARY
		h.write([]byte{telnetIAC, telnetDO, telnetOptBinary})
		got = h.readN(3)
		if got[0] != telnetIAC || got[1] != telnetWILL || got[2] != telnetOptBinary {
			h.t.Fatalf("expected WILL BINARY, got %x", got)
		}
		// Server: EraseWrite with a visible field containing HELLO.
		payload := []byte{
			cmdEraseWrite,
			wccUnlock | wccResetMDT,
			orderSBA, addrTable[0], addrTable[0],
			orderSF, 0x00,
			0xC8, 0xC5, 0xD3, 0xD3, 0xD6, // HELLO in EBCDIC
		}
		var framed []byte
		for _, b := range payload {
			framed = append(framed, b)
			if b == telnetIAC {
				framed = append(framed, telnetIAC)
			}
		}
		framed = append(framed, telnetIAC, telnetEOR)
		h.write(framed)
	})

	if emu.keyboardLock {
		t.Error("keyboard should be unlocked after EraseWrite with unlock WCC")
	}
	if !strings.Contains(emu.GetScreen(), "HELLO") {
		t.Errorf("expected HELLO on screen, got:\n%s", emu.GetScreen())
	}
	if emu.BytesIn() == 0 {
		t.Error("BytesIn should have incremented from host messages")
	}
	if emu.BytesOut() == 0 {
		t.Error("BytesOut should have incremented from WILL responses")
	}
}

// In production the injected dialer is k6's state.Dialer, which is how
// --blocked-hostnames and DNS overrides reach this extension's dials.
func TestEmulatorConnectUsesInjectedDialer(t *testing.T) {
	captured := struct {
		called  bool
		network string
		addr    string
	}{}
	stub := dialerFunc(func(ctx context.Context, network, addr string) (net.Conn, error) {
		captured.called = true
		captured.network = network
		captured.addr = addr
		// Hand back a closed pipe so handshake fails fast — we only assert
		// that the dialer was invoked.
		c1, c2 := net.Pipe()
		go func() { _ = c1.Close() }()
		return c2, nil
	})
	emu := NewEmulator()
	emu.dialer = stub

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = emu.Connect(ctx, "example.invalid", 23, time.Second)

	if !captured.called {
		t.Fatal("injected dialer was not called")
	}
	if captured.network != "tcp" {
		t.Errorf("network = %q, want \"tcp\"", captured.network)
	}
	if captured.addr != "example.invalid:23" {
		t.Errorf("addr = %q, want host:port form", captured.addr)
	}
}

type dialerFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func (f dialerFunc) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return f(ctx, network, addr)
}

func TestEmulatorReadMessageEnforcesSizeCap(t *testing.T) {
	host, client := newFakeHost(t)
	emu := NewEmulator()
	emu.conn = client
	emu.reader = bufio.NewReader(client)

	go func() {
		// Chunked so net.Pipe doesn't deadlock on one giant write; never IAC EOR.
		chunk := make([]byte, 4096)
		for i := 0; i < 70; i++ {
			if _, err := host.conn.Write(chunk); err != nil {
				return
			}
		}
	}()

	_, err := emu.readMessage()
	if err == nil {
		t.Fatal("expected readMessage to fail with oversize record")
	}
	if !strings.Contains(err.Error(), "maximum size") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEmulatorSendRecordCountsBytes(t *testing.T) {
	host, client := newFakeHost(t)
	emu := NewEmulator()
	emu.conn = client
	emu.reader = bufio.NewReader(client)

	// Concurrent drain so sendRecord doesn't block on the synchronous pipe.
	recvDone := make(chan []byte, 1)
	go func() {
		recvDone <- host.drainEOR()
	}()

	payload := []byte{aidEnter, 0x00, 0x00, 0xFF, 0x01}
	if err := emu.sendRecord(payload); err != nil {
		t.Fatalf("sendRecord: %v", err)
	}

	// drainEOR unescapes IAC IAC → 0xFF, so received payload == sent payload.
	got := <-recvDone
	if string(got) != string(payload) {
		t.Errorf("decoded payload mismatch:\n got  %x\n want %x", got, payload)
	}
	// 5 payload + 1 IAC escape + 2 IAC EOR.
	if emu.BytesOut() != 8 {
		t.Errorf("BytesOut = %d, want 8 (payload + escape + IAC EOR)", emu.BytesOut())
	}
}
