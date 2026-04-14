package tn3270

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"
)

// These tests bypass the real modules.VU by exercising the janitor select
// loop directly — the Client struct fields it touches are package-internal.

func TestJanitorDisconnectsOnContextCancel(t *testing.T) {
	_, server := net.Pipe()
	emu := NewEmulator()
	emu.conn = server
	emu.reader = bufio.NewReader(server)
	emu.connState = connTN3270

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		metrics:     &tn3270Metrics{},
		connected:   true,
		emu:         emu,
		janitorStop: make(chan struct{}),
	}
	// Mirrors the goroutine doConnect spawns; isolated from the full VU here.
	stop := c.janitorStop
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Disconnect()
		case <-stop:
		}
	}()

	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !c.IsConnected() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("janitor did not Disconnect within 2s of ctx cancel")
}

func TestJanitorStopsOnExplicitDisconnect(t *testing.T) {
	_, server := net.Pipe()
	emu := NewEmulator()
	emu.conn = server
	emu.reader = bufio.NewReader(server)
	emu.connState = connTN3270

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &Client{
		metrics:     &tn3270Metrics{},
		connected:   true,
		emu:         emu,
		janitorStop: make(chan struct{}),
	}
	stop := c.janitorStop
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-stop:
		}
		close(done)
	}()

	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("janitor did not exit after explicit Disconnect")
	}
}
