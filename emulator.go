package tn3270

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

type TraceLogger interface {
	Printf(format string, args ...interface{})
}

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
	telnetOptSGA      byte = 0x03 // Suppress Go-Ahead
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
	attrProtected  byte = 0x20
	attrNumeric    byte = 0x10
	attrDisplay    byte = 0x0C // bits 3-2: display characteristics
	attrNonDisplay byte = 0x0C // when bits 3-2 = 11: non-display (hidden)
	attrMDT        byte = 0x01
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

// Extended attribute type identifiers (as used in SFE/SA/MF type-value pairs).
const (
	extAttrBasic     byte = 0xC0 // 3270 field attribute
	extAttrHighlight byte = 0x41 // Extended highlighting
	extAttrColor     byte = 0x42 // Foreground color
	extAttrBgColor   byte = 0x45 // Background color
	extAttrCharSet   byte = 0x43 // Character set
	extAttrTransp    byte = 0x46 // Transparency
)

// Reply mode values for WSF Set Reply Mode.
const (
	replyModeField    byte = 0x00
	replyModeExtField byte = 0x01
	replyModeChar     byte = 0x02
)

// Connection states.
const (
	connNotConnected = iota
	connPending      // TCP connected, telnet negotiation in progress
	connTN3270       // Plain TN3270 mode
	connTN3270E      // TN3270E mode
)

// TN3270E function codes (RFC 2355 section 7).
const (
	tn3270eFuncBindImage     byte = 0x00
	tn3270eFuncDataStreamCtl byte = 0x01
	tn3270eFuncResponses     byte = 0x02
	tn3270eFuncSCSCtlCodes   byte = 0x03
	tn3270eFuncSysReq        byte = 0x04
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

// Terminal model definitions.
type termModel struct {
	name    string // Terminal type string (e.g., "IBM-3278-2-E")
	rows    int
	cols    int
	altRows int
	altCols int
}

var termModels = map[int]termModel{
	2: {name: "IBM-3278-2", rows: 24, cols: 80, altRows: 24, altCols: 80},
	3: {name: "IBM-3278-3", rows: 24, cols: 80, altRows: 32, altCols: 80},
	4: {name: "IBM-3278-4", rows: 24, cols: 80, altRows: 43, altCols: 80},
	5: {name: "IBM-3278-5", rows: 24, cols: 80, altRows: 27, altCols: 132},
}

// TN3270E subnegotiation constants (RFC 2355).
const (
	tn3270eSend       byte = 0x08
	tn3270eDeviceType byte = 0x02
	tn3270eFunctions  byte = 0x03
	tn3270eRequest    byte = 0x07
	tn3270eIs         byte = 0x04

	// TN3270E data types in the 5-byte header.
	tn3270eData3270    byte = 0x00
	tn3270eSCSData     byte = 0x01
	tn3270eResponse    byte = 0x02
	tn3270eBindImage   byte = 0x03
	tn3270eUnbind      byte = 0x04
	tn3270eNVTData     byte = 0x05
	tn3270eRequest3270 byte = 0x06
	tn3270eSSCPLUData  byte = 0x07
)

// extAttrs holds extended attributes for a single screen position.
type extAttrs struct {
	highlight byte // Extended highlighting (blink, reverse, underscore, etc.)
	color     byte // Foreground color
	bgColor   byte // Background color
	charSet   byte // Character set (LCID)
	ge        bool // True if this position was written via Graphic Escape
}

// Redeclared locally to keep the emulator package free of the k6 lib import;
// k6's state.Dialer satisfies it structurally.
type dialContexter interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type Emulator struct {
	conn   net.Conn
	reader *bufio.Reader

	// nil means dial through a raw net.Dialer (used by unit tests).
	dialer dialContexter

	// Screen buffer — dynamically allocated based on model/alternate size
	buffer     []byte     // EBCDIC character data
	fieldAttrs []byte     // Field attribute at field start positions
	isAttr     []bool     // True if position is a field attribute
	extFields  []extAttrs // Extended attributes per position

	cursorAddr   int  // Current cursor position
	bufAddr      int  // Current buffer address during data stream processing
	keyboardLock bool // Keyboard locked by host
	lastAID      byte // Last AID sent to the host

	// Screen dimensions
	rows    int  // Current rows (may be default or alternate)
	cols    int  // Current cols
	size    int  // rows * cols
	model   int  // Terminal model number (2-5)
	defRows int  // Default screen rows
	defCols int  // Default screen cols
	altRows int  // Alternate screen rows
	altCols int  // Alternate screen cols
	useAlt  bool // True if alternate screen size is active

	// Connection state
	connState int

	// Reply mode: field (0x00), extended field (0x01), character (0x02)
	replyMode     byte
	replyModeAttr []byte // Attribute types requested in character mode

	// Character mode SA tracking — current values set by SA orders
	curHighlight byte
	curColor     byte
	curBgColor   byte
	curCharSet   byte

	// TN3270E state
	tn3270e          bool    // True when TN3270E mode is active
	tn3270eResponses bool    // True if RESPONSES function was negotiated
	seqNum           uint16  // Outgoing sequence number for TN3270E headers
	tn3270eRespHdr   [5]byte // Last received TN3270E header

	// TLS
	useTLS           bool
	tlsInsecure      bool
	tlsServerName    string
	tlsMinVersion    uint16
	tlsRootCAs       *x509.CertPool
	tlsClientCrts    []tls.Certificate
	tlsCipherSuites  []uint16
	tlsBase          *tls.Config // clone target (typically k6 vu state TLSConfig)

	// Code page
	codePage *CodePage

	// Debug tracing
	trace  bool
	logger TraceLogger

	// Atomic so concurrent metric readers don't race the I/O goroutines.
	bytesIn  atomic.Int64
	bytesOut atomic.Int64
}

// 0xFE never occurs in a non-display EBCDIC field and stands out in hex dumps.
const redactByte byte = 0xFE

// 256 KiB is ~70× a model-5 screen but bounds a hostile peer that refuses
// to send IAC EOR.
const maxRecordSize = 256 * 1024

func (e *Emulator) BytesIn() int64  { return e.bytesIn.Load() }
func (e *Emulator) BytesOut() int64 { return e.bytesOut.Load() }

func (e *Emulator) write(p []byte) (int, error) {
	n, err := e.conn.Write(p)
	if n > 0 {
		e.bytesOut.Add(int64(n))
	}
	return n, err
}

// Counting at the reader layer means every byte (including telnet framing) is
// counted once, regardless of which branch of the telnet handler consumes it.
func (e *Emulator) readByte() (byte, error) {
	b, err := e.reader.ReadByte()
	if err == nil {
		e.bytesIn.Add(1)
	}
	return b, err
}

func NewEmulator() *Emulator {
	return newEmulatorModel(2)
}

func NewEmulatorModel(model int) *Emulator {
	if _, ok := termModels[model]; !ok {
		model = 2
	}
	return newEmulatorModel(model)
}

// logf writes a trace log message if tracing is enabled.
func (e *Emulator) logf(format string, args ...interface{}) {
	if e.trace && e.logger != nil {
		e.logger.Printf(format, args...)
	}
}

func newEmulatorModel(model int) *Emulator {
	tm := termModels[model]
	defSize := tm.rows * tm.cols
	e := &Emulator{
		model:        model,
		defRows:      tm.rows,
		defCols:      tm.cols,
		altRows:      tm.altRows,
		altCols:      tm.altCols,
		rows:         tm.rows,
		cols:         tm.cols,
		size:         defSize,
		keyboardLock: true,
		lastAID:      aidNone,
		connState:    connNotConnected,
		replyMode:    replyModeField,
	}
	maxSize := tm.altRows * tm.altCols
	if defSize > maxSize {
		maxSize = defSize
	}
	e.buffer = make([]byte, maxSize)
	e.fieldAttrs = make([]byte, maxSize)
	e.isAttr = make([]bool, maxSize)
	e.extFields = make([]extAttrs, maxSize)
	e.logger = log.Default()
	return e
}

// Cloning tlsBase (k6's vu.State().TLSConfig) lets --cacerts, SSLKEYLOGFILE,
// and runtime cipher policy flow through; per-call options override the clone.
func (e *Emulator) buildTLSConfig(host string) *tls.Config {
	var cfg *tls.Config
	if e.tlsBase != nil {
		cfg = e.tlsBase.Clone()
	} else {
		cfg = &tls.Config{}
	}
	cfg.ServerName = host
	if e.tlsServerName != "" {
		cfg.ServerName = e.tlsServerName
	}
	cfg.InsecureSkipVerify = e.tlsInsecure //#nosec G402 -- user-controlled option for self-signed certs
	if e.tlsMinVersion != 0 {
		cfg.MinVersion = e.tlsMinVersion
	}
	// Without this, Go's zero MinVersion permits TLS 1.0 / 1.1.
	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS12
	}
	if e.tlsRootCAs != nil {
		cfg.RootCAs = e.tlsRootCAs
	}
	if len(e.tlsClientCrts) > 0 {
		cfg.Certificates = e.tlsClientCrts
	}
	if len(e.tlsCipherSuites) > 0 {
		cfg.CipherSuites = e.tlsCipherSuites
	}
	return cfg
}

// Telnet negotiation and the initial screen read happen synchronously, before return.
func (e *Emulator) Connect(ctx context.Context, host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	dialCtx, dialCancel := context.WithTimeout(ctx, timeout)
	defer dialCancel()

	var dialer dialContexter
	if e.dialer != nil {
		dialer = e.dialer
	} else {
		dialer = &net.Dialer{}
	}

	rawConn, err := dialer.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("tcp connection failed: %w", err)
	}

	conn := rawConn
	if e.useTLS {
		// Dial first via the k6 dialer (so blocked-hostname policy applies
		// even to TLS), then layer TLS. Handshake uses the dial deadline.
		tlsConn := tls.Client(rawConn, e.buildTLSConfig(host))
		if err := tlsConn.HandshakeContext(dialCtx); err != nil {
			_ = rawConn.Close()
			return fmt.Errorf("tls handshake failed: %w", err)
		}
		conn = tlsConn
	}

	e.conn = conn
	e.reader = bufio.NewReader(conn)
	e.keyboardLock = true
	e.connState = connPending
	e.eraseScreen()

	if err := e.handshake(ctx, timeout); err != nil {
		e.closeConn()
		return err
	}
	return nil
}

func (e *Emulator) handshake(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for e.keyboardLock {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("timeout during initial handshake")
		}
		if err := e.conn.SetReadDeadline(time.Now().Add(min(remaining, 500*time.Millisecond))); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}
		msg, err := e.readMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return fmt.Errorf("failed during initial handshake: %w", err)
		}
		if err := e.processAndRespond(msg); err != nil {
			return fmt.Errorf("failed to process initial screen: %w", err)
		}
	}

	_ = e.conn.SetReadDeadline(time.Time{})
	return nil
}

func (e *Emulator) closeConn() {
	if e.conn != nil {
		_ = e.conn.Close()
		e.conn = nil
	}
	e.connState = connNotConnected
}

// Wipes the screen buffer so credentials don't sit in process memory waiting
// for Go's GC after the session ends.
func (e *Emulator) Disconnect() {
	e.closeConn()
	e.wipeBuffers()
}

func (e *Emulator) wipeBuffers() {
	for i := range e.buffer {
		e.buffer[i] = 0
	}
	for i := range e.fieldAttrs {
		e.fieldAttrs[i] = 0
	}
	for i := range e.isAttr {
		e.isAttr[i] = false
	}
	for i := range e.extFields {
		e.extFields[i] = extAttrs{}
	}
	e.cursorAddr = 0
	e.bufAddr = 0
	e.lastAID = 0
	e.keyboardLock = true
	e.replyMode = replyModeField
	e.replyModeAttr = nil
	e.tn3270e = false
	e.tn3270eResponses = false
	e.seqNum = 0
	e.tn3270eRespHdr = [5]byte{}
}

func (e *Emulator) IsConnected() bool {
	return e.conn != nil && e.connState >= connTN3270
}

// readMessage reads a complete 3270 data stream message, processing
// telnet commands inline. Returns the raw 3270 data (without telnet framing).
// In TN3270E mode, the 5-byte header is stored in tn3270eRespHdr and stripped.
// Returns nil data for non-3270 TN3270E data types.
func (e *Emulator) readMessage() ([]byte, error) {
	var data []byte
	for {
		b, err := e.readByte()
		if err != nil {
			return nil, err
		}
		if len(data) >= maxRecordSize {
			return nil, fmt.Errorf("record exceeds maximum size of %d bytes", maxRecordSize)
		}

		if b == telnetIAC {
			next, err := e.readByte()
			if err != nil {
				return nil, err
			}

			switch next {
			case telnetIAC:
				data = append(data, telnetIAC) // Escaped 0xFF
			case telnetEOR:
				if e.tn3270e {
					if len(data) >= 5 {
						copy(e.tn3270eRespHdr[:], data[:5])
						data = data[5:]
						dataType := e.tn3270eRespHdr[0]
						if e.trace {
							dtName := fmt.Sprintf("0x%02X", dataType)
							switch dataType {
							case tn3270eData3270:
								dtName = "3270-DATA"
							case tn3270eSCSData:
								dtName = "SCS-DATA"
							case tn3270eResponse:
								dtName = "RESPONSE"
							case tn3270eBindImage:
								dtName = "BIND-IMAGE"
							case tn3270eUnbind:
								dtName = "UNBIND"
							case tn3270eNVTData:
								dtName = "NVT-DATA"
							case tn3270eSSCPLUData:
								dtName = "SSCP-LU-DATA"
							}
							e.logf("[TN3270E] RECV header: type=%s req=0x%02X resp=0x%02X seq=%d datalen=%d",
								dtName, e.tn3270eRespHdr[1], e.tn3270eRespHdr[2],
								int(e.tn3270eRespHdr[3])<<8|int(e.tn3270eRespHdr[4]), len(data))
						}
						switch dataType {
						case tn3270eData3270:
							// Fall through to return data
						case tn3270eSSCPLUData:
							// SSCP-LU data: treat as displayable text
							return data, nil
						default:
							// Non-3270 data type (SCS, BIND, UNBIND, NVT, etc.)
							return nil, nil
						}
					} else {
						if e.trace {
							e.logf("[TN3270E] RECV short record (%d bytes), skipping", len(data))
						}
						// Short TN3270E record (< 5 bytes); skip
						return nil, nil
					}
				}
				return data, nil // End of record
			case telnetDO:
				opt, err := e.readByte()
				if err != nil {
					return nil, err
				}
				if err := e.handleTelnetDo(opt); err != nil {
					return nil, err
				}
			case telnetDONT:
				opt, err := e.readByte()
				if err != nil {
					return nil, err
				}
				if err := e.sendTelnet(telnetIAC, telnetWONT, opt); err != nil {
					return nil, err
				}
			case telnetWILL:
				opt, err := e.readByte()
				if err != nil {
					return nil, err
				}
				if err := e.handleTelnetWill(opt); err != nil {
					return nil, err
				}
			case telnetWONT:
				opt, err := e.readByte()
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
	if e.tn3270e && e.tn3270eResponses && e.tn3270eRespHdr[2] == 0x02 {
		return e.sendTN3270EPositiveResponse()
	}
	return nil
}

func (e *Emulator) telnetOptName(opt byte) string {
	switch opt {
	case telnetOptBinary:
		return "BINARY"
	case telnetOptSGA:
		return "SGA"
	case telnetOptTermType:
		return "TERMINAL-TYPE"
	case telnetOptEOR:
		return "EOR"
	case telnetOptTN3270E:
		return "TN3270E"
	default:
		return fmt.Sprintf("0x%02X", opt)
	}
}

func (e *Emulator) handleTelnetDo(opt byte) error {
	switch opt {
	case telnetOptTermType, telnetOptEOR, telnetOptBinary, telnetOptTN3270E, telnetOptSGA:
		if e.trace {
			e.logf("[TN3270] RECV DO %s → SEND WILL %s", e.telnetOptName(opt), e.telnetOptName(opt))
		}
		return e.sendTelnet(telnetIAC, telnetWILL, opt)
	default:
		if e.trace {
			e.logf("[TN3270] RECV DO %s → SEND WONT %s", e.telnetOptName(opt), e.telnetOptName(opt))
		}
		return e.sendTelnet(telnetIAC, telnetWONT, opt)
	}
}

func (e *Emulator) handleTelnetWill(opt byte) error {
	switch opt {
	case telnetOptEOR, telnetOptBinary, telnetOptTN3270E, telnetOptSGA:
		if e.trace {
			e.logf("[TN3270] RECV WILL %s → SEND DO %s", e.telnetOptName(opt), e.telnetOptName(opt))
		}
		return e.sendTelnet(telnetIAC, telnetDO, opt)
	default:
		if e.trace {
			e.logf("[TN3270] RECV WILL %s → SEND DONT %s", e.telnetOptName(opt), e.telnetOptName(opt))
		}
		return e.sendTelnet(telnetIAC, telnetDONT, opt)
	}
}

func (e *Emulator) handleSubnegotiation() error {
	// Real telnet subnegotiations are tens of bytes; cap defends against a
	// server that streams without IAC SE.
	const maxSubnegBytes = 4 * 1024
	var subData []byte
	for {
		b, err := e.readByte()
		if err != nil {
			return err
		}
		if len(subData) >= maxSubnegBytes {
			return fmt.Errorf("subnegotiation exceeds maximum size of %d bytes", maxSubnegBytes)
		}
		if b == telnetIAC {
			next, err := e.readByte()
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
		tm := termModels[e.model]
		if e.trace {
			e.logf("[TN3270] RECV SB TERMINAL-TYPE SEND → SEND IS %s", tm.name)
		}
		resp := []byte{telnetIAC, telnetSB, telnetOptTermType, telnetTermTypeIs}
		resp = append(resp, []byte(tm.name)...)
		resp = append(resp, telnetIAC, telnetSE)
		_, err := e.write(resp)
		return err
	}

	if len(subData) >= 1 && subData[0] == telnetOptTN3270E {
		return e.handleTN3270ESub(subData[1:])
	}

	if e.trace {
		e.logf("[TN3270] RECV SB unknown: %s", hex.EncodeToString(subData))
	}
	return nil
}

func (e *Emulator) handleTN3270ESub(data []byte) error {
	if len(data) < 1 {
		return nil
	}

	switch data[0] {
	case tn3270eSend:
		// SEND DEVICE-TYPE: respond with DEVICE-TYPE REQUEST <term-type>
		if len(data) >= 2 && data[1] == tn3270eDeviceType {
			tm := termModels[e.model]
			if e.trace {
				e.logf("[TN3270E] RECV SEND DEVICE-TYPE → REQUEST %s", tm.name)
			}
			resp := []byte{telnetIAC, telnetSB, telnetOptTN3270E, tn3270eDeviceType, tn3270eRequest}
			resp = append(resp, []byte(tm.name)...)
			resp = append(resp, telnetIAC, telnetSE)
			_, err := e.write(resp)
			return err
		}

	case tn3270eDeviceType:
		if len(data) < 2 {
			return nil
		}
		switch data[1] {
		case tn3270eIs:
			// DEVICE-TYPE IS: server acknowledged device type.
			// Client must now send FUNCTIONS REQUEST to complete TN3270E negotiation.
			// Request RESPONSES function for proper acknowledgement support.
			if e.trace {
				e.logf("[TN3270E] RECV DEVICE-TYPE IS → SEND FUNCTIONS REQUEST [RESPONSES]")
			}
			resp := []byte{telnetIAC, telnetSB, telnetOptTN3270E, tn3270eFunctions, tn3270eRequest,
				tn3270eFuncResponses,
				telnetIAC, telnetSE}
			_, err := e.write(resp)
			return err
		case 0x06: // REJECT
			// Server rejected our device type or TN3270E entirely.
			// Fall back to plain TN3270 by sending WONT TN3270E.
			if e.trace {
				e.logf("[TN3270E] RECV DEVICE-TYPE REJECT → falling back to TN3270")
			}
			e.connState = connTN3270
			return e.sendTelnet(telnetIAC, telnetWONT, telnetOptTN3270E)
		}
		return nil

	case tn3270eFunctions:
		if len(data) < 2 {
			return nil
		}
		switch data[1] {
		case tn3270eIs:
			// FUNCTIONS IS: server accepted. Parse which functions are active.
			e.tn3270e = true
			e.connState = connTN3270E
			e.tn3270eResponses = false
			for _, f := range data[2:] {
				if f == tn3270eFuncResponses {
					e.tn3270eResponses = true
				}
			}
			if e.trace {
				e.logf("[TN3270E] RECV FUNCTIONS IS (responses=%v) → TN3270E active", e.tn3270eResponses)
			}
		case tn3270eRequest:
			// Server sent FUNCTIONS REQUEST with its preferred list.
			// Intersect with what we support (only RESPONSES).
			var accepted []byte
			for _, f := range data[2:] {
				if f == tn3270eFuncResponses {
					accepted = append(accepted, f)
					e.tn3270eResponses = true
				}
			}
			if e.trace {
				e.logf("[TN3270E] RECV FUNCTIONS REQUEST → SEND FUNCTIONS IS (responses=%v)", e.tn3270eResponses)
			}
			resp := []byte{telnetIAC, telnetSB, telnetOptTN3270E, tn3270eFunctions, tn3270eIs}
			resp = append(resp, accepted...)
			resp = append(resp, telnetIAC, telnetSE)
			if _, err := e.write(resp); err != nil {
				return err
			}
			e.tn3270e = true
			e.connState = connTN3270E
		case 0x06: // REJECT
			// Fall back to plain TN3270.
			if e.trace {
				e.logf("[TN3270E] RECV FUNCTIONS REJECT → falling back to TN3270")
			}
			e.connState = connTN3270
			return e.sendTelnet(telnetIAC, telnetWONT, telnetOptTN3270E)
		}
	}

	return nil
}

func (e *Emulator) sendTelnet(data ...byte) error {
	_, err := e.write(data)
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
	_, err := e.write(buf.Bytes())
	return err
}

// sendData sends 3270 data framed with IAC EOR, escaping any 0xFF bytes.
// In TN3270E mode, a 5-byte header is prepended.
func (e *Emulator) sendData(data []byte) error {
	if e.trace {
		aidName := fmt.Sprintf("0x%02X", data[0])
		switch data[0] {
		case aidEnter:
			aidName = "Enter"
		case aidClear:
			aidName = "Clear"
		case aidSF:
			aidName = "SF(QueryReply)"
		}
		redacted := e.redactOutbound(data)
		if len(redacted) <= 256 {
			e.logf("[3270] SEND aid=%s len=%d data=%s", aidName, len(data), hex.EncodeToString(redacted))
		} else {
			e.logf("[3270] SEND aid=%s len=%d data=%s... (%d bytes)", aidName, len(data), hex.EncodeToString(redacted[:256]), len(data))
		}
	}
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

// Walks the record as the host would (AID + cursor + orders + data) and masks
// any data byte landing in a non-display field. On a malformed/truncated order
// the remainder of the record is masked rather than risk leaking cleartext.
func (e *Emulator) redactOutbound(data []byte) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	if e.size == 0 || len(data) == 0 {
		return out
	}
	// Query replies describe terminal capability; no user data to mask.
	if data[0] == aidSF {
		return out
	}
	// Skip AID + 2-byte cursor address.
	i := 3
	if i > len(data) {
		return out
	}
	bufAddr := 0
	redactTail := func(from int) {
		for j := from; j < len(out); j++ {
			out[j] = redactByte
		}
	}
	advance := func() {
		if e.isFieldNonDisplay(bufAddr) {
			out[i] = redactByte
		}
		bufAddr = (bufAddr + 1) % e.size
		i++
	}
	for i < len(data) {
		switch data[i] {
		case orderSBA:
			if i+2 >= len(data) {
				redactTail(i)
				return out
			}
			bufAddr = decodeAddr(data[i+1], data[i+2])
			if bufAddr < 0 || bufAddr >= e.size {
				bufAddr = 0
			}
			i += 3
		case orderSF:
			if i+1 >= len(data) {
				redactTail(i)
				return out
			}
			i += 2
		case orderSFE:
			if i+1 >= len(data) {
				redactTail(i)
				return out
			}
			n := int(data[i+1])
			skip := 2 + 2*n
			if i+skip > len(data) {
				redactTail(i)
				return out
			}
			i += skip
		case orderSA, orderMF:
			if i+2 >= len(data) {
				redactTail(i)
				return out
			}
			i += 3
		case orderIC, orderPT:
			i++
		case orderRA:
			if i+3 >= len(data) {
				redactTail(i)
				return out
			}
			i += 4
		case orderEUA:
			if i+2 >= len(data) {
				redactTail(i)
				return out
			}
			i += 3
		case orderGE:
			if i+1 >= len(data) {
				redactTail(i)
				return out
			}
			// GE escape: the following byte is data, placed at bufAddr.
			i++
			advance()
		default:
			advance()
		}
	}
	return out
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

	if e.trace {
		cmdName := fmt.Sprintf("0x%02X", cmd)
		switch cmd {
		case cmdWrite, cmdWriteSNA:
			cmdName = "Write"
		case cmdEraseWrite, cmdEraseWriteSNA:
			cmdName = "EraseWrite"
		case cmdEraseWriteAlt, cmdEraseWriteAltSNA:
			cmdName = "EraseWriteAlt"
		case cmdWSF, cmdWSFSNA:
			cmdName = "WSF"
		case cmdReadBuffer, cmdReadBufferSNA:
			cmdName = "ReadBuffer"
		case cmdReadMod, cmdReadModSNA:
			cmdName = "ReadModified"
		case cmdReadModAll, cmdReadModAllSNA:
			cmdName = "ReadModifiedAll"
		case cmdEraseAllUnp, cmdEraseAllUnpSNA:
			cmdName = "EraseAllUnprotected"
		}
		e.logf("[3270] RECV cmd=%s len=%d", cmdName, len(data))
		if len(data) <= 256 {
			e.logf("[3270] DATA %s", hex.EncodeToString(data))
		} else {
			e.logf("[3270] DATA %s... (%d bytes)", hex.EncodeToString(data[:256]), len(data))
		}
	}

	switch cmd {
	case cmdEraseWrite, cmdEraseWriteSNA:
		if len(data) < 2 {
			return fmt.Errorf("eraseWrite: missing WCC byte")
		}
		e.switchScreenSize(false)
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
		e.switchScreenSize(true)
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
		// Unknown or unimplemented command (e.g., NOP 0x03).
		// Ignore gracefully — real z/OS hosts send commands that
		// a load testing emulator does not need to act on.
		return nil
	}
}

// switchScreenSize switches between default and alternate screen sizes.
func (e *Emulator) switchScreenSize(alt bool) {
	if alt && (e.altRows != e.defRows || e.altCols != e.defCols) {
		e.rows = e.altRows
		e.cols = e.altCols
		e.useAlt = true
	} else {
		e.rows = e.defRows
		e.cols = e.defCols
		e.useAlt = false
	}
	e.size = e.rows * e.cols
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
		if e.trace {
			e.logf("[3270] WCC keyboard restore → unlocked")
		}
		e.keyboardLock = false
		if e.connState == connPending {
			if e.tn3270e {
				e.connState = connTN3270E
			} else {
				e.connState = connTN3270
			}
		}
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
			e.extFields[e.bufAddr] = extAttrs{}
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
			ea := extAttrs{}
			for j := 0; j < count; j++ {
				attrType := data[i+2+j*2]
				attrVal := data[i+2+j*2+1]
				switch attrType {
				case extAttrBasic:
					attr = attrVal
				case extAttrHighlight:
					ea.highlight = attrVal
				case extAttrColor:
					ea.color = attrVal
				case extAttrBgColor:
					ea.bgColor = attrVal
				case extAttrCharSet:
					ea.charSet = attrVal
				}
			}
			e.isAttr[e.bufAddr] = true
			e.fieldAttrs[e.bufAddr] = attr
			e.buffer[e.bufAddr] = 0x00
			e.extFields[e.bufAddr] = ea
			e.bufAddr = (e.bufAddr + 1) % e.size
			i += needed

		case orderSA:
			if i+2 >= len(data) {
				return fmt.Errorf("sa: insufficient data at offset %d", i)
			}
			attrType := data[i+1]
			attrVal := data[i+2]
			switch attrType {
			case extAttrHighlight:
				e.curHighlight = attrVal
			case extAttrColor:
				e.curColor = attrVal
			case extAttrBgColor:
				e.curBgColor = attrVal
			case extAttrCharSet:
				e.curCharSet = attrVal
			}
			i += 3

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
				attrType := data[i+2+j*2]
				attrVal := data[i+2+j*2+1]
				switch attrType {
				case extAttrBasic:
					if e.isAttr[e.bufAddr] {
						e.fieldAttrs[e.bufAddr] = attrVal
					}
				case extAttrHighlight:
					e.extFields[e.bufAddr].highlight = attrVal
				case extAttrColor:
					e.extFields[e.bufAddr].color = attrVal
				case extAttrBgColor:
					e.extFields[e.bufAddr].bgColor = attrVal
				case extAttrCharSet:
					e.extFields[e.bufAddr].charSet = attrVal
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
			// Check for GE variant: if the byte after RA address is orderGE, the
			// fill character follows it and should be marked as GE.
			isGE := false
			if fillChar == orderGE && i+4 < len(data) {
				fillChar = data[i+4]
				isGE = true
				i++ // consume extra byte
			}
			for e.bufAddr != endAddr {
				e.buffer[e.bufAddr] = fillChar
				if isGE {
					e.extFields[e.bufAddr].ge = true
				}
				e.applyCurrentSA(e.bufAddr)
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
					e.extFields[e.bufAddr] = extAttrs{}
				}
				e.bufAddr = (e.bufAddr + 1) % e.size
			}
			i += 3

		case orderGE:
			if i+1 >= len(data) {
				return fmt.Errorf("ge: missing character at offset %d", i)
			}
			e.buffer[e.bufAddr] = data[i+1]
			e.extFields[e.bufAddr].ge = true
			e.applyCurrentSA(e.bufAddr)
			e.bufAddr = (e.bufAddr + 1) % e.size
			i += 2

		default:
			// Data byte: place in buffer
			e.buffer[e.bufAddr] = b
			e.extFields[e.bufAddr].ge = false
			e.applyCurrentSA(e.bufAddr)
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

// applyCurrentSA stores the current SA-set attributes to a position.
func (e *Emulator) applyCurrentSA(pos int) {
	if e.curHighlight != 0 {
		e.extFields[pos].highlight = e.curHighlight
	}
	if e.curColor != 0 {
		e.extFields[pos].color = e.curColor
	}
	if e.curBgColor != 0 {
		e.extFields[pos].bgColor = e.curBgColor
	}
	if e.curCharSet != 0 {
		e.extFields[pos].charSet = e.curCharSet
	}
}

// resetCurrentSA resets current SA values (called at start of write commands).
func (e *Emulator) resetCurrentSA() {
	e.curHighlight = 0
	e.curColor = 0
	e.curBgColor = 0
	e.curCharSet = 0
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
			e.handleSetReplyMode(sfData)
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

// handleSetReplyMode processes WSF Set Reply Mode.
func (e *Emulator) handleSetReplyMode(data []byte) {
	if len(data) < 2 {
		return
	}
	// data[0] = partition ID, data[1] = mode
	mode := data[1]
	if mode <= replyModeChar {
		e.replyMode = mode
	}
	// In character mode, remaining bytes are the attribute types to include
	if mode == replyModeChar && len(data) > 2 {
		e.replyModeAttr = make([]byte, len(data)-2)
		copy(e.replyModeAttr, data[2:])
	} else {
		e.replyModeAttr = nil
	}
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
	// Report alternate screen size if model supports it, otherwise default
	usableRows := e.altRows
	usableCols := e.altCols
	usableSize := usableRows * usableCols
	e.appendQR(&buf, qrUsableArea, []byte{
		0x01,                                                    // 12/14-bit addressing
		0x00,                                                    // no variable cells
		byte((usableCols >> 8) & 0xFF), byte(usableCols & 0xFF), // usable width
		byte((usableRows >> 8) & 0xFF), byte(usableRows & 0xFF), // usable height
		0x01,                                                    // units: characters
		byte((usableCols >> 8) & 0xFF), byte(usableCols & 0xFF), // Xr
		byte((usableRows >> 8) & 0xFF), byte(usableRows & 0xFF), // Yr
		0x07,                                                    // AW: default cell width
		0x0C,                                                    // AH: default cell height
		byte((usableSize >> 8) & 0xFF), byte(usableSize & 0xFF), // buffer size
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
		0x0B,                                                  // length of implicit partition entry
		0x01,                                                  // implicit partition
		0x00,                                                  // reserved
		byte((e.defCols >> 8) & 0xFF), byte(e.defCols & 0xFF), // default width
		byte((e.defRows >> 8) & 0xFF), byte(e.defRows & 0xFF), // default height
		byte((e.altCols >> 8) & 0xFF), byte(e.altCols & 0xFF), // alternate width
		byte((e.altRows >> 8) & 0xFF), byte(e.altRows & 0xFF), // alternate height
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
// Response format depends on the current reply mode.
func (e *Emulator) sendReadBuffer() error {
	var buf bytes.Buffer
	buf.WriteByte(e.lastAID)
	addr := encodeAddr(e.cursorAddr)
	buf.Write(addr[:])

	switch e.replyMode {
	case replyModeField:
		for i := 0; i < e.size; i++ {
			if e.isAttr[i] {
				buf.WriteByte(orderSF)
				buf.WriteByte(e.fieldAttrs[i])
			} else {
				buf.WriteByte(e.buffer[i])
			}
		}
	case replyModeExtField:
		for i := 0; i < e.size; i++ {
			if e.isAttr[i] {
				e.writeSFE(&buf, i)
			} else {
				if e.extFields[i].ge {
					buf.WriteByte(orderGE)
				}
				buf.WriteByte(e.buffer[i])
			}
		}
	case replyModeChar:
		e.writeCharacterModeBuffer(&buf)
	}

	return e.sendData(buf.Bytes())
}

// writeSFE writes an SFE order with field attribute and any extended attributes.
func (e *Emulator) writeSFE(buf *bytes.Buffer, pos int) {
	ea := e.extFields[pos]
	pairs := []struct {
		typ byte
		val byte
	}{
		{extAttrBasic, e.fieldAttrs[pos]},
	}
	if ea.highlight != 0 {
		pairs = append(pairs, struct{ typ, val byte }{extAttrHighlight, ea.highlight})
	}
	if ea.color != 0 {
		pairs = append(pairs, struct{ typ, val byte }{extAttrColor, ea.color})
	}
	if ea.bgColor != 0 {
		pairs = append(pairs, struct{ typ, val byte }{extAttrBgColor, ea.bgColor})
	}
	if ea.charSet != 0 {
		pairs = append(pairs, struct{ typ, val byte }{extAttrCharSet, ea.charSet})
	}

	buf.WriteByte(orderSFE)
	buf.WriteByte(byte(len(pairs) & 0xFF))
	for _, p := range pairs {
		buf.WriteByte(p.typ)
		buf.WriteByte(p.val)
	}
}

// writeCharacterModeBuffer writes the buffer in character mode, interleaving
// SA orders whenever extended attributes change.
func (e *Emulator) writeCharacterModeBuffer(buf *bytes.Buffer) {
	var lastHL, lastFG, lastBG, lastCS byte

	for i := 0; i < e.size; i++ {
		if e.isAttr[i] {
			e.writeSFE(buf, i)
			// Reset SA tracking at field boundaries
			ea := e.extFields[i]
			lastHL = ea.highlight
			lastFG = ea.color
			lastBG = ea.bgColor
			lastCS = ea.charSet
			continue
		}

		ea := e.extFields[i]

		// Emit SA orders for changed attributes
		if ea.highlight != lastHL {
			buf.WriteByte(orderSA)
			buf.WriteByte(extAttrHighlight)
			buf.WriteByte(ea.highlight)
			lastHL = ea.highlight
		}
		if ea.color != lastFG {
			buf.WriteByte(orderSA)
			buf.WriteByte(extAttrColor)
			buf.WriteByte(ea.color)
			lastFG = ea.color
		}
		if ea.bgColor != lastBG {
			buf.WriteByte(orderSA)
			buf.WriteByte(extAttrBgColor)
			buf.WriteByte(ea.bgColor)
			lastBG = ea.bgColor
		}
		if ea.charSet != lastCS {
			buf.WriteByte(orderSA)
			buf.WriteByte(extAttrCharSet)
			buf.WriteByte(ea.charSet)
			lastCS = ea.charSet
		}

		if ea.ge {
			buf.WriteByte(orderGE)
		}
		buf.WriteByte(e.buffer[i])
	}
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
				e.writeFieldDataByte(buf, i)
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
		var geFlags []bool
		cur := dataStart
		for {
			if e.isAttr[cur] {
				break
			}
			fieldData = append(fieldData, e.buffer[cur])
			geFlags = append(geFlags, e.extFields[cur].ge)
			cur = (cur + 1) % e.size
			if cur == dataStart {
				break
			}
		}

		// Strip trailing nulls and spaces
		for len(fieldData) > 0 && (fieldData[len(fieldData)-1] == 0x00 || fieldData[len(fieldData)-1] == 0x40) && !geFlags[len(geFlags)-1] {
			fieldData = fieldData[:len(fieldData)-1]
			geFlags = geFlags[:len(geFlags)-1]
		}

		if len(fieldData) > 0 {
			addr := encodeAddr(dataStart)
			buf.WriteByte(orderSBA)
			buf.Write(addr[:])
			for j, b := range fieldData {
				if geFlags[j] {
					buf.WriteByte(orderGE)
				}
				buf.WriteByte(b)
			}
		}
	}
}

func (e *Emulator) eraseScreen() {
	for i := 0; i < e.size; i++ {
		e.buffer[i] = 0x00
		e.fieldAttrs[i] = 0
		e.isAttr[i] = false
		e.extFields[i] = extAttrs{}
	}
	e.cursorAddr = 0
	e.bufAddr = 0
	e.replyMode = replyModeField
	e.replyModeAttr = nil
	e.resetCurrentSA()
}

func (e *Emulator) eraseUnprotected() {
	for i := 0; i < e.size; i++ {
		if !e.isAttr[i] && !e.isFieldProtectedAt(i) {
			e.buffer[i] = 0x00
			e.extFields[i] = extAttrs{}
		}
		if e.isAttr[i] && e.fieldAttrs[i]&attrProtected == 0 {
			e.fieldAttrs[i] &^= attrMDT
		}
	}
	e.keyboardLock = false
}

// isFieldNonDisplay returns true if the field at pos has non-display attribute.
// Non-display fields (like password fields) have display bits 3-2 = 11.
func (e *Emulator) isFieldNonDisplay(pos int) bool {
	attrPos := e.findFieldAttr(pos)
	if attrPos < 0 {
		return false
	}
	return e.fieldAttrs[attrPos]&attrDisplay == attrNonDisplay
}

// GetScreen returns the screen buffer as ASCII text.
func (e *Emulator) GetScreen() string {
	e2a := &ebcdicToASCII
	if e.codePage != nil {
		e2a = &e.codePage.EBCDICToASCII
	}
	var buf strings.Builder
	buf.Grow(e.size + e.rows)
	for row := 0; row < e.rows; row++ {
		for col := 0; col < e.cols; col++ {
			pos := row*e.cols + col
			if e.isAttr[pos] {
				buf.WriteByte(' ')
			} else if e.isFieldNonDisplay(pos) {
				buf.WriteByte(' ')
			} else {
				ch := e.buffer[pos]
				if ch == 0x00 {
					buf.WriteByte(' ')
				} else {
					a := e2a[ch]
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
	a2e := &asciiToEBCDIC
	if e.codePage != nil {
		a2e = &e.codePage.ASCIIToEBCDIC
	}
	for _, ch := range []byte(text) {
		if e.isAttr[e.cursorAddr] {
			// Skip field attribute and advance to data position
			e.cursorAddr = (e.cursorAddr + 1) % e.size
		}

		if e.isFieldProtectedAt(e.cursorAddr) {
			return fmt.Errorf("cannot type in protected field at position %d", e.cursorAddr)
		}

		e.buffer[e.cursorAddr] = a2e[ch]
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
func (e *Emulator) sendAID(ctx context.Context, aid byte, timeout time.Duration) error {
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
	return e.readUntilUnlocked(ctx, timeout)
}

// writeFieldDataByte writes a single field data byte, prefixing with GE if needed.
func (e *Emulator) writeFieldDataByte(buf *bytes.Buffer, pos int) {
	if e.extFields[pos].ge {
		buf.WriteByte(orderGE)
	}
	buf.WriteByte(e.buffer[pos])
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
				e.writeFieldDataByte(buf, i)
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
		var geFlags []bool
		cur := dataStart
		for {
			if e.isAttr[cur] {
				break
			}
			fieldData = append(fieldData, e.buffer[cur])
			geFlags = append(geFlags, e.extFields[cur].ge)
			cur = (cur + 1) % e.size
			if cur == dataStart {
				break
			}
		}

		// Strip trailing nulls and spaces (but not if GE-flagged).
		// Nulls (0x00) are always stripped per 3270 protocol.
		// Spaces (0x40) from host RA fill are stripped to match real terminal behavior.
		for len(fieldData) > 0 && (fieldData[len(fieldData)-1] == 0x00 || fieldData[len(fieldData)-1] == 0x40) && !geFlags[len(geFlags)-1] {
			fieldData = fieldData[:len(fieldData)-1]
			geFlags = geFlags[:len(geFlags)-1]
		}

		if len(fieldData) > 0 {
			addr := encodeAddr(dataStart)
			buf.WriteByte(orderSBA)
			buf.Write(addr[:])
			for j, b := range fieldData {
				if geFlags[j] {
					buf.WriteByte(orderGE)
				}
				buf.WriteByte(b)
			}
		}
	}
}

// readUntilUnlocked reads 3270 messages until the keyboard is unlocked.
func (e *Emulator) readUntilUnlocked(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	defer func() {
		_ = e.conn.SetReadDeadline(time.Time{})
	}()

	for e.keyboardLock {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("timeout waiting for host response")
		}
		if err := e.conn.SetReadDeadline(time.Now().Add(min(remaining, 500*time.Millisecond))); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}
		msg, err := e.readMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return fmt.Errorf("failed to read host response: %w", err)
		}
		if err := e.processAndRespond(msg); err != nil {
			return fmt.Errorf("failed to process host response: %w", err)
		}
	}
	return nil
}

// WaitForField waits until the keyboard is unlocked and an input field is available.
func (e *Emulator) WaitForField(ctx context.Context, timeout time.Duration) error {
	if e.HasUnlockedField() {
		return nil
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if err := e.conn.SetReadDeadline(time.Now().Add(min(remaining, 500*time.Millisecond))); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}
		msg, err := e.readMessage()
		_ = e.conn.SetReadDeadline(time.Time{})
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
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
func (e *Emulator) Enter(ctx context.Context, timeout time.Duration) error {
	return e.sendAID(ctx, aidEnter, timeout)
}

// PF sends a PF key (1-24).
func (e *Emulator) PF(ctx context.Context, key int, timeout time.Duration) error {
	return e.sendAID(ctx, pfAIDs[key-1], timeout)
}

// PA sends a PA key (1-3).
func (e *Emulator) PA(ctx context.Context, key int, timeout time.Duration) error {
	return e.sendAID(ctx, paAIDs[key-1], timeout)
}

// Clear clears the screen and sends the Clear AID.
func (e *Emulator) Clear(ctx context.Context, timeout time.Duration) error {
	e.eraseScreen()
	return e.sendAID(ctx, aidClear, timeout)
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
