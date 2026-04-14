package tn3270

import (
	"bytes"
	"testing"
)

// Password fields (attrDisplay bits = 11) must never appear in trace output.
func TestRedactOutboundMasksNonDisplayField(t *testing.T) {
	emu := NewEmulator()

	// Simulate a screen with two fields:
	//   pos 0: field attribute for an unprotected visible field starting at pos 1
	//   pos 10: field attribute for an unprotected NON-DISPLAY field starting at pos 11
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = 0x00 // unprotected, visible
	emu.isAttr[10] = true
	emu.fieldAttrs[10] = attrNonDisplay // unprotected, non-display

	// Hand-built outbound record: AID Enter, cursor addr (0,0), then
	//   SBA pos 1 + 'U' 'S' 'R' (visible field — should NOT be redacted)
	//   SBA pos 11 + 'P' 'W' 'D' (non-display — MUST be redacted)
	sbaVis := encodeAddr(1)
	sbaPw := encodeAddr(11)
	record := []byte{
		aidEnter,
		0x00, 0x00, // cursor address (arbitrary)
		orderSBA, sbaVis[0], sbaVis[1],
		asciiToEBCDIC['U'], asciiToEBCDIC['S'], asciiToEBCDIC['R'],
		orderSBA, sbaPw[0], sbaPw[1],
		asciiToEBCDIC['P'], asciiToEBCDIC['W'], asciiToEBCDIC['D'],
	}

	redacted := emu.redactOutbound(record)

	for i, want := range []byte{asciiToEBCDIC['U'], asciiToEBCDIC['S'], asciiToEBCDIC['R']} {
		if redacted[6+i] != want {
			t.Errorf("visible byte %d modified: got 0x%02X, want 0x%02X", i, redacted[6+i], want)
		}
	}
	for i := 12; i <= 14; i++ {
		if redacted[i] != redactByte {
			t.Errorf("password byte at %d not redacted: got 0x%02X, want 0x%02X", i, redacted[i], redactByte)
		}
	}
}

func TestRedactOutboundLeavesQueryReplyAlone(t *testing.T) {
	emu := NewEmulator()
	// Query replies carry no user input; their structured-field bytes should
	// pass through even if some incidentally match non-display field positions.
	record := []byte{aidSF, 0x00, 0x05, 0x00, 0x07, 0x81, 0x80, 0x81}
	redacted := emu.redactOutbound(record)
	if !bytes.Equal(record, redacted) {
		t.Errorf("QueryReply payload was modified:\norig = %x\ngot  = %x", record, redacted)
	}
}

func TestRedactOutboundHandlesTruncatedOrder(t *testing.T) {
	emu := NewEmulator()
	// A truncated SBA must not leak the rest of the record cleartext.
	record := []byte{
		aidEnter, 0x00, 0x00,
		orderSBA, // intentionally missing 2 address bytes
	}
	redacted := emu.redactOutbound(record)
	if !bytes.Equal(redacted[:3], record[:3]) {
		t.Errorf("header modified")
	}
	if redacted[3] != redactByte {
		t.Errorf("truncated order byte not redacted: got 0x%02X", redacted[3])
	}
}

func TestRedactOutboundHandlesEmptyAndShort(t *testing.T) {
	emu := NewEmulator()
	// Records too short to contain orders should pass through unchanged.
	for _, r := range [][]byte{nil, {}, {aidEnter}, {aidEnter, 0x00}} {
		got := emu.redactOutbound(r)
		if !bytes.Equal(r, got) {
			t.Errorf("short record %x modified to %x", r, got)
		}
	}
}

func TestRedactOutboundGEEscape(t *testing.T) {
	emu := NewEmulator()
	// GE escapes the next byte as data; when that lands in a non-display
	// field the data byte (not the GE marker itself) must be masked.
	emu.isAttr[0] = true
	emu.fieldAttrs[0] = attrNonDisplay
	sba := encodeAddr(1)
	record := []byte{
		aidEnter, 0x00, 0x00,
		orderSBA, sba[0], sba[1],
		orderGE, 0xC1, // GE + 'A' at pos 1 (non-display)
	}
	redacted := emu.redactOutbound(record)
	if redacted[6] != orderGE {
		t.Errorf("GE marker should be preserved, got 0x%02X", redacted[6])
	}
	if redacted[7] != redactByte {
		t.Errorf("GE-escaped data byte should be redacted, got 0x%02X", redacted[7])
	}
}
