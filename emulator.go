package tn3270

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"strings"
	"time"
)

// Telnet protocol constants.
const (
	telnetIAC  byte = 0xFF
	telnetDO   byte = 0xFD
	telnetDONT byte = 0xFE
	telnetWILL byte = 0xFB
	telnetWONT byte = 0xFC
	telnetSB   byte = 0xFA
	telnetSE   byte = 0xF0
	telnetEOR  byte = 0xEF

	telnetOptBinary   byte = 0x00
	telnetOptTermType byte = 0x18
	telnetOptEOR      byte = 0x19
	telnetOptTN3270E  byte = 0x28

	telnetTermTypeSend byte = 0x01
	telnetTermTypeIs   byte = 0x00
)

// 3270 data stream command codes.
const (
	cmdWrite         byte = 0xF1
	cmdEraseWrite    byte = 0xF5
	cmdEraseWriteAlt byte = 0x7E
	cmdWSF           byte = 0xF3
	cmdReadBuffer    byte = 0xF2
	cmdReadMod       byte = 0xF6
	cmdReadModAll    byte = 0x6E
	cmdEraseAllUnp   byte = 0x6F

	// SNA variants (used in TN3270E and Outbound 3270DS).
	cmdWriteSNA         byte = 0x01
	cmdReadBufferSNA    byte = 0x02
	cmdEraseWriteSNA    byte = 0x05
	cmdReadModSNA       byte = 0x06
	cmdEraseWriteAltSNA byte = 0x0D
	cmdReadModAllSNA    byte = 0x0E
	cmdEraseAllUnpSNA   byte = 0x0F
	cmdWSFSNA           byte = 0x11
)

// 3270 orders.
const (
	orderSBA byte = 0x11 // Set Buffer Address
	orderSF  byte = 0x1D // Start Field
	orderSFE byte = 0x29 // Start Field Extended
	orderSA  byte = 0x28 // Set Attribute
	orderMF  byte = 0x2C // Modify Field
	orderIC  byte = 0x13 // Insert Cursor
	orderPT  byte = 0x05 // Program Tab
	orderRA  byte = 0x3C // Repeat to Address
	orderEUA byte = 0x12 // Erase Unprotected to Address
	orderGE  byte = 0x08 // Graphic Escape
)

// Field attribute bits.
const (
	attrProtected byte = 0x20
	attrNumeric   byte = 0x10
	attrMDT       byte = 0x01
)

// WCC (Write Control Character) bits.
const (
	wccUnlock   byte = 0x02
	wccResetMDT byte = 0x01
)

// AID (Attention Identifier) bytes.
const (
	aidNone  byte = 0x60
	aidEnter byte = 0x7D
	aidClear byte = 0x6D
	aidSF    byte = 0x88 // Structured Field AID for query replies
)

// WSF structured field types.
const (
	sfReadPartition  byte = 0x01
	sfEraseReset     byte = 0x03
	sfSetReplyMode   byte = 0x09
	sfOutbound3270DS byte = 0x40
)

// Query Reply codes.
const (
	qrSummary      byte = 0x80
	qrUsableArea   byte = 0x81
	qrCharSets     byte = 0x85
	qrColor        byte = 0x86
	qrHighlighting byte = 0x87
	qrReplyModes   byte = 0x88
	qrImplicitPart byte = 0xA6
)

// Supported query reply codes, used in the Summary reply.
var supportedQRCodes = []byte{
	qrSummary, qrUsableArea, qrCharSets, qrColor,
	qrHighlighting, qrReplyModes, qrImplicitPart,
}

var pfAIDs = [24]byte{
	0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, // PF1-9
	0x7A, 0x7B, 0x7C, // PF10-12
	0xC1, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9, // PF13-21
	0x4A, 0x4B, 0x4C, // PF22-24
}

var paAIDs = [3]byte{0x6C, 0x6E, 0x6B}

// Buffer address encoding table (6-bit values to EBCDIC bytes).
var addrTable = [64]byte{
	0x40, 0xC1, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7,
	0xC8, 0xC9, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F,
	0x50, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7,
	0xD8, 0xD9, 0x5A, 0x5B, 0x5C, 0x5D, 0x5E, 0x5F,
	0x60, 0x61, 0xE2, 0xE3, 0xE4, 0xE5, 0xE6, 0xE7,
	0xE8, 0xE9, 0x6A, 0x6B, 0x6C, 0x6D, 0x6E, 0x6F,
	0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7,
	0xF8, 0xF9, 0x7A, 0x7B, 0x7C, 0x7D, 0x7E, 0x7F,
}

// Reverse lookup: EBCDIC byte → 6-bit value.
var addrReverse [256]byte

func init() {
	for i, v := range addrTable {
		addrReverse[v] = byte(i)
	}
}

func encodeAddr(addr int) [2]byte {
	return [2]byte{
		addrTable[(addr>>6)&0x3F],
		addrTable[addr&0x3F],
	}
}

func decodeAddr(b1, b2 byte) int {
	if b1&0xC0 == 0x00 {
		// 14-bit address
		return (int(b1)&0x3F)<<8 | int(b2)
	}
	// 12-bit address
	return int(addrReverse[b1])<<6 | int(addrReverse[b2])
}

// TN3270E subnegotiation constants (RFC 2355).
const (
	tn3270eSend       byte = 0x08
	tn3270eDeviceType byte = 0x02
	tn3270eFunctions  byte = 0x03
	tn3270eRequest    byte = 0x07
	tn3270eIs         byte = 0x04

	// TN3270E data types in the 5-byte header.
	tn3270eData3270 byte = 0x00
	tn3270eResponse byte = 0x02
)

// Emulator implements a native TN3270 terminal emulator.
type Emulator struct {
	conn   net.Conn
	reader *bufio.Reader

	// Screen buffer
	buffer     [1920]byte // EBCDIC character data
	fieldAttrs [1920]byte // Field attribute at field start positions
	isAttr     [1920]bool // True if position is a field attribute

	cursorAddr   int  // Current cursor position
	bufAddr      int  // Current buffer address during data stream processing
	keyboardLock bool // Keyboard locked by host
	lastAID      byte // Last AID sent to the host

	rows int
	cols int
	size int

	// TN3270E state
	tn3270e        bool    // True when TN3270E mode is active
	seqNum         uint16  // Outgoing sequence number for TN3270E headers
	tn3270eRespHdr [5]byte // Last received TN3270E header
}

func NewEmulator() *Emulator {
	return &Emulator{
		rows:         24,
		cols:         80,
		size:         1920,
		keyboardLock: true,
		lastAID:      aidNone,
	}
}

// Connect establishes a TN3270 connection to the given host and port.
// Telnet negotiation and initial screen reading happen synchronously.
func (e *Emulator) Connect(host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("tcp connection failed: %w", err)
	}

	e.conn = conn
	e.reader = bufio.NewReader(conn)
	e.keyboardLock = true
	e.eraseScreen()

	if err := e.handshake(timeout); err != nil {
		e.closeConn()
		return err
	}
	return nil
}

func (e *Emulator) handshake(timeout time.Duration) error {
	if err := e.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	for e.keyboardLock {
		msg, err := e.readMessage()
		if err != nil {
			return fmt.Errorf("failed during initial handshake: %w", err)
		}
		if err := e.processAndRespond(msg); err != nil {
			return fmt.Errorf("failed to process initial screen: %w", err)
		}
	}

	if err := e.conn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("failed to clear read deadline: %w", err)
	}
	return nil
}

// closeConn closes and nils the connection, ignoring close errors.
func (e *Emulator) closeConn() {
	if e.conn != nil {
		_ = e.conn.Close()
		e.conn = nil
	}
}

// Disconnect closes the TN3270 connection.
func (e *Emulator) Disconnect() {
	e.closeConn()
}

// IsConnected returns true if the emulator has an active connection.
func (e *Emulator) IsConnected() bool {
	return e.conn != nil
}

// readMessage reads a complete 3270 data stream message, processing
// telnet commands inline. Returns the raw 3270 data (without telnet framing).
// In TN3270E mode, the 5-byte header is stored in tn3270eRespHdr and stripped.
// Returns nil data for non-3270 TN3270E data types.
func (e *Emulator) readMessage() ([]byte, error) {
	var data []byte
	for {
		b, err := e.reader.ReadByte()
		if err != nil {
			return nil, err
		}

		if b == telnetIAC {
			next, err := e.reader.ReadByte()
			if err != nil {
				return nil, err
			}

			switch next {
			case telnetIAC:
				data = append(data, telnetIAC) // Escaped 0xFF
			case telnetEOR:
				if e.tn3270e && len(data) >= 5 {
					copy(e.tn3270eRespHdr[:], data[:5])
					data = data[5:]
					if e.tn3270eRespHdr[0] != tn3270eData3270 {
						return nil, nil // Non-3270 data type
					}
				}
				return data, nil // End of record
			case telnetDO:
				opt, err := e.reader.ReadByte()
				if err != nil {
					return nil, err
				}
				if err := e.handleTelnetDo(opt); err != nil {
					return nil, err
				}
			case telnetDONT:
				opt, err := e.reader.ReadByte()
				if err != nil {
					return nil, err
				}
				if err := e.sendTelnet(telnetIAC, telnetWONT, opt); err != nil {
					return nil, err
				}
			case telnetWILL:
				opt, err := e.reader.ReadByte()
				if err != nil {
					return nil, err
				}
				if err := e.handleTelnetWill(opt); err != nil {
					return nil, err
				}
			case telnetWONT:
				opt, err := e.reader.ReadByte()
				if err != nil {
					return nil, err
				}
				if err := e.sendTelnet(telnetIAC, telnetDONT, opt); err != nil {
					return nil, err
				}
			case telnetSB:
				if err := e.handleSubnegotiation(); err != nil {
					return nil, err
				}
			default:
				// Other telnet command, ignore
			}
		} else {
			data = append(data, b)
		}
	}
}

// processAndRespond processes a 3270 message and handles TN3270E
// positive response acknowledgement if the host requested one.
func (e *Emulator) processAndRespond(msg []byte) error {
	if len(msg) > 0 {
		if err := e.processMessage(msg); err != nil {
			return err
		}
	}
	if e.tn3270e && e.tn3270eRespHdr[2] == 0x02 {
		return e.sendTN3270EPositiveResponse()
	}
	return nil
}

func (e *Emulator) handleTelnetDo(opt byte) error {
	switch opt {
	case telnetOptTermType, telnetOptEOR, telnetOptBinary, telnetOptTN3270E:
		return e.sendTelnet(telnetIAC, telnetWILL, opt)
	default:
		return e.sendTelnet(telnetIAC, telnetWONT, opt)
	}
}

func (e *Emulator) handleTelnetWill(opt byte) error {
	switch opt {
	case telnetOptEOR, telnetOptBinary, telnetOptTN3270E:
		return e.sendTelnet(telnetIAC, telnetDO, opt)
	default:
		return e.sendTelnet(telnetIAC, telnetDONT, opt)
	}
}

func (e *Emulator) handleSubnegotiation() error {
	var subData []byte
	for {
		b, err := e.reader.ReadByte()
		if err != nil {
			return err
		}
		if b == telnetIAC {
			next, err := e.reader.ReadByte()
			if err != nil {
				return err
			}
			if next == telnetSE {
				break
			}
			if next == telnetIAC {
				subData = append(subData, telnetIAC)
			}
		} else {
			subData = append(subData, b)
		}
	}

	if len(subData) >= 2 && subData[0] == telnetOptTermType && subData[1] == telnetTermTypeSend {
		resp := []byte{telnetIAC, telnetSB, telnetOptTermType, telnetTermTypeIs}
		resp = append(resp, []byte("IBM-3278-2")...)
		resp = append(resp, telnetIAC, telnetSE)
		_, err := e.conn.Write(resp)
		return err
	}

	if len(subData) >= 1 && subData[0] == telnetOptTN3270E {
		return e.handleTN3270ESub(subData[1:])
	}

	return nil
}

func (e *Emulator) handleTN3270ESub(data []byte) error {
	if len(data) < 1 {
		return nil
	}

	switch data[0] {
	case tn3270eSend:
		// SEND DEVICE-TYPE: respond with DEVICE-TYPE REQUEST IBM-3278-2
		if len(data) >= 2 && data[1] == tn3270eDeviceType {
			resp := []byte{telnetIAC, telnetSB, telnetOptTN3270E, tn3270eDeviceType, tn3270eRequest}
			resp = append(resp, []byte("IBM-3278-2")...)
			resp = append(resp, telnetIAC, telnetSE)
			_, err := e.conn.Write(resp)
			return err
		}

	case tn3270eDeviceType:
		// DEVICE-TYPE IS: server acknowledged device type.
		// Client must now send FUNCTIONS REQUEST to complete TN3270E negotiation.
		if len(data) >= 2 && data[1] == tn3270eIs {
			resp := []byte{telnetIAC, telnetSB, telnetOptTN3270E, tn3270eFunctions, tn3270eRequest, telnetIAC, telnetSE}
			_, err := e.conn.Write(resp)
			return err
		}
		return nil

	case tn3270eFunctions:
		// FUNCTIONS IS: server accepted our functions request. TN3270E is now active.
		if len(data) >= 2 && data[1] == tn3270eIs {
			e.tn3270e = true
			return nil
		}
	}

	return nil
}

func (e *Emulator) sendTelnet(data ...byte) error {
	_, err := e.conn.Write(data)
	return err
}

// sendRecord writes a telnet record: escapes 0xFF bytes and appends IAC EOR.
func (e *Emulator) sendRecord(data []byte) error {
	var buf bytes.Buffer
	for _, b := range data {
		buf.WriteByte(b)
		if b == telnetIAC {
			buf.WriteByte(telnetIAC)
		}
	}
	buf.WriteByte(telnetIAC)
	buf.WriteByte(telnetEOR)
	_, err := e.conn.Write(buf.Bytes())
	return err
}

// sendData sends 3270 data framed with IAC EOR, escaping any 0xFF bytes.
// In TN3270E mode, a 5-byte header is prepended.
func (e *Emulator) sendData(data []byte) error {
	if e.tn3270e {
		header := [5]byte{tn3270eData3270, 0x00, 0x00, byte((e.seqNum >> 8) & 0xFF), byte(e.seqNum & 0xFF)}
		e.seqNum++
		record := make([]byte, 5+len(data))
		copy(record, header[:])
		copy(record[5:], data)
		return e.sendRecord(record)
	}
	return e.sendRecord(data)
}

// sendTN3270EPositiveResponse sends a TN3270E positive response for the
// last received message, using its sequence number.
func (e *Emulator) sendTN3270EPositiveResponse() error {
	seqHi := e.tn3270eRespHdr[3]
	seqLo := e.tn3270eRespHdr[4]
	// Header: data_type=RESPONSE, request=0, response=0, seq from received message.
	// Body: 0x00 0x00 = positive device-end.
	return e.sendRecord([]byte{tn3270eResponse, 0x00, 0x00, seqHi, seqLo, 0x00, 0x00})
}

// processMessage handles a complete 3270 data stream message.
// WCC processing uses two phases per the 3270 Data Stream spec:
// Reset MDT happens before orders, keyboard restore happens after.
func (e *Emulator) processMessage(data []byte) error {
	if len(data) < 1 {
		return nil
	}

	cmd := data[0]

	switch cmd {
	case cmdEraseWrite, cmdEraseWriteSNA:
		if len(data) < 2 {
			return fmt.Errorf("eraseWrite: missing WCC byte")
		}
		e.eraseScreen()
		wcc := data[1]
		e.resetMDT(wcc)
		e.bufAddr = 0
		if len(data) > 2 {
			if err := e.processOrders(data[2:]); err != nil {
				return err
			}
		}
		e.restoreKeyboard(wcc)
		return nil

	case cmdWrite, cmdWriteSNA:
		if len(data) < 2 {
			return fmt.Errorf("write: missing WCC byte")
		}
		wcc := data[1]
		e.resetMDT(wcc)
		e.bufAddr = 0
		if len(data) > 2 {
			if err := e.processOrders(data[2:]); err != nil {
				return err
			}
		}
		e.restoreKeyboard(wcc)
		return nil

	case cmdEraseWriteAlt, cmdEraseWriteAltSNA:
		if len(data) < 2 {
			return fmt.Errorf("eraseWriteAlt: missing WCC byte")
		}
		e.eraseScreen()
		wcc := data[1]
		e.resetMDT(wcc)
		e.bufAddr = 0
		if len(data) > 2 {
			if err := e.processOrders(data[2:]); err != nil {
				return err
			}
		}
		e.restoreKeyboard(wcc)
		return nil

	case cmdEraseAllUnp, cmdEraseAllUnpSNA:
		e.eraseUnprotected()
		return nil

	case cmdWSF, cmdWSFSNA:
		return e.processWSF(data[1:])

	case cmdReadBuffer, cmdReadBufferSNA:
		return e.sendReadBuffer()

	case cmdReadMod, cmdReadModSNA:
		return e.sendReadModified(false)

	case cmdReadModAll, cmdReadModAllSNA:
		return e.sendReadModified(true)

	default:
		return fmt.Errorf("unknown 3270 command: 0x%02X", cmd)
	}
}

// resetMDT clears the MDT bit on all fields if the WCC requests it.
// This runs BEFORE orders/data are processed.
func (e *Emulator) resetMDT(wcc byte) {
	if wcc&wccResetMDT != 0 {
		for i := 0; i < e.size; i++ {
			if e.isAttr[i] {
				e.fieldAttrs[i] &^= attrMDT
			}
		}
	}
}

// restoreKeyboard unlocks the keyboard if the WCC requests it.
// This runs AFTER orders/data are processed so the screen is complete
// before user input is allowed.
func (e *Emulator) restoreKeyboard(wcc byte) {
	if wcc&wccUnlock != 0 {
		e.keyboardLock = false
	}
}

func (e *Emulator) processOrders(data []byte) error {
	i := 0
	icSet := false

	for i < len(data) {
		b := data[i]

		switch b {
		case orderSBA:
			if i+2 >= len(data) {
				return fmt.Errorf("sba: insufficient data at offset %d", i)
			}
			e.bufAddr = decodeAddr(data[i+1], data[i+2]) % e.size
			i += 3

		case orderSF:
			if i+1 >= len(data) {
				return fmt.Errorf("sf: missing attribute at offset %d", i)
			}
			e.isAttr[e.bufAddr] = true
			e.fieldAttrs[e.bufAddr] = data[i+1]
			e.buffer[e.bufAddr] = 0x00
			e.bufAddr = (e.bufAddr + 1) % e.size
			i += 2

		case orderSFE:
			if i+1 >= len(data) {
				return fmt.Errorf("sfe: missing count at offset %d", i)
			}
			count := int(data[i+1])
			needed := 2 + count*2
			if i+needed > len(data) {
				return fmt.Errorf("sfe: insufficient data at offset %d", i)
			}
			attr := byte(0)
			for j := 0; j < count; j++ {
				if data[i+2+j*2] == 0xC0 {
					attr = data[i+2+j*2+1]
				}
			}
			e.isAttr[e.bufAddr] = true
			e.fieldAttrs[e.bufAddr] = attr
			e.buffer[e.bufAddr] = 0x00
			e.bufAddr = (e.bufAddr + 1) % e.size
			i += needed

		case orderSA:
			if i+2 >= len(data) {
				return fmt.Errorf("sa: insufficient data at offset %d", i)
			}
			i += 3 // Skip extended attribute (type + value)

		case orderMF:
			if i+1 >= len(data) {
				return fmt.Errorf("mf: missing count at offset %d", i)
			}
			count := int(data[i+1])
			needed := 2 + count*2
			if i+needed > len(data) {
				return fmt.Errorf("mf: insufficient data at offset %d", i)
			}
			for j := 0; j < count; j++ {
				if data[i+2+j*2] == 0xC0 && e.isAttr[e.bufAddr] {
					e.fieldAttrs[e.bufAddr] = data[i+2+j*2+1]
				}
			}
			i += needed

		case orderIC:
			e.cursorAddr = e.bufAddr
			icSet = true
			i++

		case orderPT:
			// Program Tab: advance to next unprotected field
			e.bufAddr = e.nextUnprotectedField(e.bufAddr)
			i++

		case orderRA:
			if i+3 >= len(data) {
				return fmt.Errorf("ra: insufficient data at offset %d", i)
			}
			endAddr := decodeAddr(data[i+1], data[i+2]) % e.size
			fillChar := data[i+3]
			for e.bufAddr != endAddr {
				e.buffer[e.bufAddr] = fillChar
				e.bufAddr = (e.bufAddr + 1) % e.size
			}
			i += 4

		case orderEUA:
			if i+2 >= len(data) {
				return fmt.Errorf("eua: insufficient data at offset %d", i)
			}
			endAddr := decodeAddr(data[i+1], data[i+2]) % e.size
			for e.bufAddr != endAddr {
				if !e.isAttr[e.bufAddr] && !e.isFieldProtectedAt(e.bufAddr) {
					e.buffer[e.bufAddr] = 0x00
				}
				e.bufAddr = (e.bufAddr + 1) % e.size
			}
			i += 3

		case orderGE:
			if i+1 >= len(data) {
				return fmt.Errorf("ge: missing character at offset %d", i)
			}
			e.buffer[e.bufAddr] = data[i+1]
			e.bufAddr = (e.bufAddr + 1) % e.size
			i += 2

		default:
			// Data byte: place in buffer
			e.buffer[e.bufAddr] = b
			e.bufAddr = (e.bufAddr + 1) % e.size
			i++
		}
	}

	// If IC was not set, cursor follows the last buffer address
	if !icSet {
		e.cursorAddr = e.bufAddr
	}

	return nil
}

// processWSF handles Write Structured Field commands.
func (e *Emulator) processWSF(data []byte) error {
	i := 0
	for i < len(data) {
		if i+2 > len(data) {
			break
		}
		sfLen := int(data[i])<<8 | int(data[i+1])
		if sfLen == 0 {
			sfLen = len(data) - i // Extends to end of record
		}
		if sfLen < 3 || i+sfLen > len(data) {
			break
		}
		sfID := data[i+2]
		sfData := data[i+3 : i+sfLen]

		switch sfID {
		case sfReadPartition:
			if err := e.handleReadPartition(sfData); err != nil {
				return err
			}
		case sfEraseReset:
			e.eraseScreen()
			e.keyboardLock = false
		case sfSetReplyMode:
			// We only support field mode; acknowledge but no state change needed
		case sfOutbound3270DS:
			if len(sfData) >= 2 {
				// sfData[0] is partition ID; sfData[1:] is 3270 command stream
				if err := e.processMessage(sfData[1:]); err != nil {
					return err
				}
			}
		}

		i += sfLen
	}
	return nil
}

// handleReadPartition dispatches Read Partition operations.
func (e *Emulator) handleReadPartition(data []byte) error {
	if len(data) < 2 {
		return nil
	}
	// data[0] = partition ID (0xFF = default)
	op := data[1]

	switch op {
	case 0x02: // Query
		return e.sendQueryReply()
	case 0x03: // QueryList
		return e.sendQueryReply() // Respond with all supported replies
	case cmdReadBuffer: // Read Buffer (0xF2) within partition
		return e.sendReadBuffer()
	case cmdReadMod: // Read Modified (0xF6) within partition
		return e.sendReadModified(false)
	case cmdReadModAll: // Read Modified All (0x6E) within partition
		return e.sendReadModified(true)
	}
	return nil
}

// sendQueryReply sends a structured field query reply with all supported
// terminal capabilities. This is the response to WSF Read Partition Query.
func (e *Emulator) sendQueryReply() error {
	var buf bytes.Buffer
	buf.WriteByte(aidSF) // Structured Field AID

	// Summary: list of all supported QCODEs
	e.appendQR(&buf, qrSummary, supportedQRCodes)

	// Usable Area: screen dimensions and addressing mode
	e.appendQR(&buf, qrUsableArea, []byte{
		0x01,                                            // 12/14-bit addressing
		0x00,                                            // no variable cells
		byte((e.cols >> 8) & 0xFF), byte(e.cols & 0xFF), // usable width
		byte((e.rows >> 8) & 0xFF), byte(e.rows & 0xFF), // usable height
		0x01,                                            // units: characters
		byte((e.cols >> 8) & 0xFF), byte(e.cols & 0xFF), // Xr
		byte((e.rows >> 8) & 0xFF), byte(e.rows & 0xFF), // Yr
		0x07,                                            // AW: default cell width
		0x0C,                                            // AH: default cell height
		byte((e.size >> 8) & 0xFF), byte(e.size & 0xFF), // buffer size
	})

	// Character Sets: CP037, GE-capable
	e.appendQR(&buf, qrCharSets, []byte{
		0x82,                   // flags: GE + CGCSGID present
		0x00,                   // more flags
		0x00,                   // SDW (use default)
		0x00,                   // SDH (use default)
		0x00, 0x00, 0x00, 0x00, // Load PS format types
		0x07, // descriptor length = 7
		// Character set descriptor: base (CP037)
		0x00,       // SET: 0
		0x10,       // FLAGS: non-loadable
		0x00,       // LCID: 0
		0x02, 0xB9, // CGCSGID char set: 697
		0x00, 0x25, // CGCSGID code page: 037
	})

	// Color: 8 color pairs (neutral)
	e.appendQR(&buf, qrColor, []byte{
		0x00,       // flags
		0x08,       // number of color pairs
		0x00, 0xF4, // default -> green
		0xF1, 0xF1, // blue
		0xF2, 0xF2, // red
		0xF3, 0xF3, // pink
		0xF4, 0xF4, // green
		0xF5, 0xF5, // turquoise
		0xF6, 0xF6, // yellow
		0xF7, 0xF7, // neutral white
	})

	// Highlighting: 5 modes
	e.appendQR(&buf, qrHighlighting, []byte{
		0x05,       // number of pairs
		0x00, 0xF0, // default -> normal
		0xF1, 0xF1, // blink
		0xF2, 0xF2, // reverse video
		0xF4, 0xF4, // underscore
		0xF8, 0xF8, // intensify
	})

	// Reply Modes: field, extended field, character
	e.appendQR(&buf, qrReplyModes, []byte{
		0x00, // field mode
		0x01, // extended field mode
		0x02, // character mode
	})

	// Implicit Partition: default and alternate screen sizes
	e.appendQR(&buf, qrImplicitPart, []byte{
		0x00, 0x00, // reserved
		0x0B,                                            // length of implicit partition entry
		0x01,                                            // implicit partition
		0x00,                                            // reserved
		byte((e.cols >> 8) & 0xFF), byte(e.cols & 0xFF), // default width
		byte((e.rows >> 8) & 0xFF), byte(e.rows & 0xFF), // default height
		byte((e.cols >> 8) & 0xFF), byte(e.cols & 0xFF), // alternate width
		byte((e.rows >> 8) & 0xFF), byte(e.rows & 0xFF), // alternate height
	})

	return e.sendData(buf.Bytes())
}

// appendQR appends a single query reply structured field to buf.
// Length includes the 2-byte length field itself.
func (e *Emulator) appendQR(buf *bytes.Buffer, qcode byte, data []byte) {
	length := 4 + len(data) // 2(length) + 1(SF-ID 0x81) + 1(QCODE) + data
	buf.WriteByte(byte((length >> 8) & 0xFF))
	buf.WriteByte(byte(length & 0xFF))
	buf.WriteByte(0x81) // Query Reply SF ID
	buf.WriteByte(qcode)
	buf.Write(data)
}

// sendReadBuffer sends a Read Buffer response containing the entire screen.
func (e *Emulator) sendReadBuffer() error {
	var buf bytes.Buffer
	buf.WriteByte(e.lastAID)
	addr := encodeAddr(e.cursorAddr)
	buf.Write(addr[:])

	for i := 0; i < e.size; i++ {
		if e.isAttr[i] {
			buf.WriteByte(orderSF)
			buf.WriteByte(e.fieldAttrs[i])
		} else {
			buf.WriteByte(e.buffer[i])
		}
	}

	return e.sendData(buf.Bytes())
}

// sendReadModified sends a Read Modified response. If all is true,
// all fields are included regardless of MDT (Read Modified All).
func (e *Emulator) sendReadModified(all bool) error {
	var buf bytes.Buffer
	buf.WriteByte(e.lastAID)
	addr := encodeAddr(e.cursorAddr)
	buf.Write(addr[:])

	if all {
		e.writeAllFields(&buf)
	} else {
		e.writeModifiedFields(&buf)
	}

	return e.sendData(buf.Bytes())
}

// writeAllFields appends SBA + field data for every field, regardless of MDT.
func (e *Emulator) writeAllFields(buf *bytes.Buffer) {
	if !e.isFormatted() {
		// Unformatted screen: send all non-null data
		lastNonNull := -1
		for i := e.size - 1; i >= 0; i-- {
			if e.buffer[i] != 0x00 {
				lastNonNull = i
				break
			}
		}
		if lastNonNull >= 0 {
			addr := encodeAddr(0)
			buf.WriteByte(orderSBA)
			buf.Write(addr[:])
			for i := 0; i <= lastNonNull; i++ {
				buf.WriteByte(e.buffer[i])
			}
		}
		return
	}

	for i := 0; i < e.size; i++ {
		if !e.isAttr[i] {
			continue
		}

		dataStart := (i + 1) % e.size
		var fieldData []byte
		cur := dataStart
		for {
			if e.isAttr[cur] {
				break
			}
			fieldData = append(fieldData, e.buffer[cur])
			cur = (cur + 1) % e.size
			if cur == dataStart {
				break
			}
		}

		// Strip trailing nulls
		for len(fieldData) > 0 && fieldData[len(fieldData)-1] == 0x00 {
			fieldData = fieldData[:len(fieldData)-1]
		}

		if len(fieldData) > 0 {
			addr := encodeAddr(dataStart)
			buf.WriteByte(orderSBA)
			buf.Write(addr[:])
			buf.Write(fieldData)
		}
	}
}

func (e *Emulator) eraseScreen() {
	for i := 0; i < e.size; i++ {
		e.buffer[i] = 0x00
		e.fieldAttrs[i] = 0
		e.isAttr[i] = false
	}
	e.cursorAddr = 0
	e.bufAddr = 0
}

func (e *Emulator) eraseUnprotected() {
	for i := 0; i < e.size; i++ {
		if !e.isAttr[i] && !e.isFieldProtectedAt(i) {
			e.buffer[i] = 0x00
		}
		if e.isAttr[i] && e.fieldAttrs[i]&attrProtected == 0 {
			e.fieldAttrs[i] &^= attrMDT
		}
	}
	e.keyboardLock = false
}

// GetScreen returns the screen buffer as ASCII text (24 lines of 80 chars).
func (e *Emulator) GetScreen() string {
	var buf strings.Builder
	buf.Grow(e.size + e.rows)
	for row := 0; row < e.rows; row++ {
		for col := 0; col < e.cols; col++ {
			pos := row*e.cols + col
			if e.isAttr[pos] {
				buf.WriteByte(' ')
			} else {
				ch := e.buffer[pos]
				if ch == 0x00 {
					buf.WriteByte(' ')
				} else {
					a := ebcdicToASCII[ch]
					if a == 0x00 {
						buf.WriteByte(' ')
					} else {
						buf.WriteByte(a)
					}
				}
			}
		}
		if row < e.rows-1 {
			buf.WriteByte('\n')
		}
	}
	return buf.String()
}

// Field attribute helpers

// findFieldAttr returns the position of the field attribute governing pos.
// Returns -1 if no fields are defined (unformatted screen).
func (e *Emulator) findFieldAttr(pos int) int {
	p := pos
	for i := 0; i < e.size; i++ {
		if e.isAttr[p] {
			return p
		}
		p--
		if p < 0 {
			p = e.size - 1
		}
	}
	return -1
}

// isFieldProtectedAt checks if the field at pos is protected.
func (e *Emulator) isFieldProtectedAt(pos int) bool {
	attrPos := e.findFieldAttr(pos)
	if attrPos == -1 {
		return false // Unformatted screen: all positions are unprotected
	}
	return e.fieldAttrs[attrPos]&attrProtected != 0
}

// nextUnprotectedField returns the first data position of the next
// unprotected field after from. Returns 0 if none found.
func (e *Emulator) nextUnprotectedField(from int) int {
	p := (from + 1) % e.size
	for i := 0; i < e.size; i++ {
		if e.isAttr[p] && e.fieldAttrs[p]&attrProtected == 0 {
			return (p + 1) % e.size
		}
		p = (p + 1) % e.size
	}
	return 0
}

// prevUnprotectedField returns the first data position of the previous
// unprotected field before from. Returns 0 if none found.
func (e *Emulator) prevUnprotectedField(from int) int {
	p := from
	if p == 0 {
		p = e.size - 1
	} else {
		p--
	}
	for i := 0; i < e.size; i++ {
		if e.isAttr[p] && e.fieldAttrs[p]&attrProtected == 0 {
			return (p + 1) % e.size
		}
		p--
		if p < 0 {
			p = e.size - 1
		}
	}
	return 0
}

func (e *Emulator) setMDT(pos int) {
	attrPos := e.findFieldAttr(pos)
	if attrPos >= 0 {
		e.fieldAttrs[attrPos] |= attrMDT
	}
}

func (e *Emulator) isFormatted() bool {
	for i := 0; i < e.size; i++ {
		if e.isAttr[i] {
			return true
		}
	}
	return false
}

// HasUnlockedField returns true if the keyboard is unlocked and there
// is an input field available (or the screen is unformatted).
func (e *Emulator) HasUnlockedField() bool {
	if e.keyboardLock {
		return false
	}

	hasField := false
	for i := 0; i < e.size; i++ {
		if e.isAttr[i] {
			hasField = true
			if e.fieldAttrs[i]&attrProtected == 0 {
				return true
			}
		}
	}

	// Unformatted screen (no fields) allows input everywhere
	return !hasField
}

// TypeString places ASCII text into the screen buffer at the cursor position.
func (e *Emulator) TypeString(text string) error {
	for _, ch := range []byte(text) {
		if e.isAttr[e.cursorAddr] {
			// Skip field attribute and advance to data position
			e.cursorAddr = (e.cursorAddr + 1) % e.size
		}

		if e.isFieldProtectedAt(e.cursorAddr) {
			return fmt.Errorf("cannot type in protected field at position %d", e.cursorAddr)
		}

		e.buffer[e.cursorAddr] = asciiToEBCDIC[ch]
		e.setMDT(e.cursorAddr)

		next := (e.cursorAddr + 1) % e.size
		if e.isAttr[next] {
			// Hit a field boundary
			if e.fieldAttrs[next]&attrProtected != 0 {
				// Next field is protected; skip to next unprotected
				next = e.nextUnprotectedField(next)
			} else {
				// Skip the attribute byte
				next = (next + 1) % e.size
			}
		}
		e.cursorAddr = next
	}
	return nil
}

// sendAID sends an AID response (Enter, PF, PA, Clear) and reads the host reply.
func (e *Emulator) sendAID(aid byte, timeout time.Duration) error {
	e.lastAID = aid

	var buf bytes.Buffer
	buf.WriteByte(aid)

	addr := encodeAddr(e.cursorAddr)
	buf.Write(addr[:])

	// PA keys and Clear send short read (no field data)
	if aid != aidClear && aid != paAIDs[0] && aid != paAIDs[1] && aid != paAIDs[2] {
		e.writeModifiedFields(&buf)
	}

	if err := e.sendData(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to send AID: %w", err)
	}

	e.keyboardLock = true
	return e.readUntilUnlocked(timeout)
}

// writeModifiedFields appends SBA + field data for each modified field.
func (e *Emulator) writeModifiedFields(buf *bytes.Buffer) {
	if !e.isFormatted() {
		// Unformatted screen: send all non-null data
		lastNonNull := -1
		for i := e.size - 1; i >= 0; i-- {
			if e.buffer[i] != 0x00 {
				lastNonNull = i
				break
			}
		}
		if lastNonNull >= 0 {
			addr := encodeAddr(0)
			buf.WriteByte(orderSBA)
			buf.Write(addr[:])
			for i := 0; i <= lastNonNull; i++ {
				buf.WriteByte(e.buffer[i])
			}
		}
		return
	}

	// Formatted screen: send each field with MDT set
	for i := 0; i < e.size; i++ {
		if !e.isAttr[i] || e.fieldAttrs[i]&attrMDT == 0 {
			continue
		}

		dataStart := (i + 1) % e.size

		// Collect field data until next attribute
		var fieldData []byte
		cur := dataStart
		for {
			if e.isAttr[cur] {
				break
			}
			fieldData = append(fieldData, e.buffer[cur])
			cur = (cur + 1) % e.size
			if cur == dataStart {
				break
			}
		}

		// Strip trailing nulls
		for len(fieldData) > 0 && fieldData[len(fieldData)-1] == 0x00 {
			fieldData = fieldData[:len(fieldData)-1]
		}

		if len(fieldData) > 0 {
			addr := encodeAddr(dataStart)
			buf.WriteByte(orderSBA)
			buf.Write(addr[:])
			buf.Write(fieldData)
		}
	}
}

// readUntilUnlocked reads 3270 messages until the keyboard is unlocked.
func (e *Emulator) readUntilUnlocked(timeout time.Duration) error {
	if err := e.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}
	defer func() {
		_ = e.conn.SetReadDeadline(time.Time{}) // best-effort clear
	}()

	for e.keyboardLock {
		msg, err := e.readMessage()
		if err != nil {
			return fmt.Errorf("failed to read host response: %w", err)
		}
		if err := e.processAndRespond(msg); err != nil {
			return fmt.Errorf("failed to process host response: %w", err)
		}
	}
	return nil
}

// WaitForField waits until the keyboard is unlocked and an input field is available.
func (e *Emulator) WaitForField(timeout time.Duration) error {
	if e.HasUnlockedField() {
		return nil
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if err := e.conn.SetReadDeadline(time.Now().Add(remaining)); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}
		msg, err := e.readMessage()
		_ = e.conn.SetReadDeadline(time.Time{}) // best-effort clear
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return fmt.Errorf("timeout waiting for input field")
			}
			return err
		}
		if err := e.processAndRespond(msg); err != nil {
			return err
		}
		if e.HasUnlockedField() {
			return nil
		}
	}
	return fmt.Errorf("timeout waiting for input field")
}

// Enter sends the Enter AID key.
func (e *Emulator) Enter(timeout time.Duration) error {
	return e.sendAID(aidEnter, timeout)
}

// PF sends a PF key (1-24).
func (e *Emulator) PF(key int, timeout time.Duration) error {
	return e.sendAID(pfAIDs[key-1], timeout)
}

// PA sends a PA key (1-3).
func (e *Emulator) PA(key int, timeout time.Duration) error {
	return e.sendAID(paAIDs[key-1], timeout)
}

// Clear clears the screen and sends the Clear AID.
func (e *Emulator) Clear(timeout time.Duration) error {
	e.eraseScreen()
	return e.sendAID(aidClear, timeout)
}

// Tab moves the cursor to the next unprotected field.
func (e *Emulator) Tab() {
	if e.isFormatted() {
		e.cursorAddr = e.nextUnprotectedField(e.cursorAddr)
	}
}

// BackTab moves the cursor to the previous unprotected field.
func (e *Emulator) BackTab() {
	if !e.isFormatted() {
		return
	}
	attrPos := e.findFieldAttr(e.cursorAddr)
	if attrPos >= 0 {
		fieldStart := (attrPos + 1) % e.size
		if e.cursorAddr != fieldStart {
			e.cursorAddr = fieldStart
			return
		}
	}
	e.cursorAddr = e.prevUnprotectedField(e.cursorAddr)
}

// Home moves the cursor to the first unprotected field.
func (e *Emulator) Home() {
	if e.isFormatted() {
		// Search from the end to wrap to the first field on screen
		e.cursorAddr = e.nextUnprotectedField(e.size - 1)
	} else {
		e.cursorAddr = 0
	}
}

// MoveCursor sets the cursor to the given position (0-based row and col).
func (e *Emulator) MoveCursor(row, col int) {
	e.cursorAddr = row*e.cols + col
}
