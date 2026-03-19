package tn3270

// CodePage represents an EBCDIC code page with bidirectional conversion tables.
type CodePage struct {
	Name          string
	EBCDICToASCII [256]byte
	ASCIIToEBCDIC [256]byte
}

// Default code page tables (CP037) — used by the emulator.
var ebcdicToASCII [256]byte
var asciiToEBCDIC [256]byte

// Available code pages.
var CodePages map[string]*CodePage

func init() {
	CodePages = make(map[string]*CodePage)

	cp037 := buildCP037()
	CodePages["cp037"] = cp037
	CodePages["037"] = cp037

	cp1047 := buildCP1047()
	CodePages["cp1047"] = cp1047
	CodePages["1047"] = cp1047

	// Set default
	ebcdicToASCII = cp037.EBCDICToASCII
	asciiToEBCDIC = cp037.ASCIIToEBCDIC
}

// buildCodePage creates a CodePage from a mapping of EBCDIC→ASCII values.
func buildCodePage(name string, mapping map[byte]byte) *CodePage {
	cp := &CodePage{Name: name}
	for i := range cp.EBCDICToASCII {
		cp.EBCDICToASCII[i] = ' '
	}
	cp.EBCDICToASCII[0x00] = 0x00

	for e, a := range mapping {
		cp.EBCDICToASCII[e] = a
	}

	// Build reverse mapping
	for e := 0; e < 256; e++ {
		a := cp.EBCDICToASCII[e]
		if a != ' ' && a != 0x00 && cp.ASCIIToEBCDIC[a] == 0x00 {
			cp.ASCIIToEBCDIC[a] = byte(e)
		}
	}
	cp.ASCIIToEBCDIC[' '] = 0x40
	cp.ASCIIToEBCDIC[0x00] = 0x00
	return cp
}

// buildCP037 builds Code Page 037 (US/Canada).
func buildCP037() *CodePage {
	m := map[byte]byte{
		0x40: ' ',
		0x4B: '.', 0x4C: '<', 0x4D: '(', 0x4E: '+', 0x4F: '|',
		0x50: '&',
		0x5A: '!', 0x5B: '$', 0x5C: '*', 0x5D: ')', 0x5E: ';',
		0x5F: '^',
		0x60: '-', 0x61: '/',
		0x6B: ',', 0x6C: '%', 0x6D: '_', 0x6E: '>', 0x6F: '?',
		0x79: '`',
		0x7A: ':', 0x7B: '#', 0x7C: '@', 0x7D: '\'', 0x7E: '=', 0x7F: '"',
		0xA1: '~',
		0xC0: '{', 0xD0: '}', 0xE0: '\\',
	}
	// Lowercase a-i
	for i := byte(0); i < 9; i++ {
		m[0x81+i] = 'a' + i
	}
	// Lowercase j-r
	for i := byte(0); i < 9; i++ {
		m[0x91+i] = 'j' + i
	}
	// Lowercase s-z
	for i := byte(0); i < 8; i++ {
		m[0xA2+i] = 's' + i
	}
	// Uppercase A-I
	for i := byte(0); i < 9; i++ {
		m[0xC1+i] = 'A' + i
	}
	// Uppercase J-R
	for i := byte(0); i < 9; i++ {
		m[0xD1+i] = 'J' + i
	}
	// Uppercase S-Z
	for i := byte(0); i < 8; i++ {
		m[0xE2+i] = 'S' + i
	}
	// Digits 0-9
	for i := byte(0); i < 10; i++ {
		m[0xF0+i] = '0' + i
	}
	return buildCodePage("cp037", m)
}

// buildCP1047 builds Code Page 1047 (z/OS default, Latin-1/Open Systems).
// CP1047 differs from CP037 mainly in bracket/brace/tilde positions.
func buildCP1047() *CodePage {
	m := map[byte]byte{
		0x40: ' ',
		0x4B: '.', 0x4C: '<', 0x4D: '(', 0x4E: '+', 0x4F: '|',
		0x50: '&',
		0x5A: '!', 0x5B: '$', 0x5C: '*', 0x5D: ')', 0x5E: ';',
		0x5F: '^',
		0x60: '-', 0x61: '/',
		0x6B: ',', 0x6C: '%', 0x6D: '_', 0x6E: '>', 0x6F: '?',
		0x79: '`',
		0x7A: ':', 0x7B: '#', 0x7C: '@', 0x7D: '\'', 0x7E: '=', 0x7F: '"',
		// CP1047 specific mappings (differs from CP037):
		0xA1: '~',
		0xAD: '[', 0xBD: ']',
		0xC0: '{', 0xD0: '}',
		0xE0: '\\',
	}
	// Lowercase a-i
	for i := byte(0); i < 9; i++ {
		m[0x81+i] = 'a' + i
	}
	// Lowercase j-r
	for i := byte(0); i < 9; i++ {
		m[0x91+i] = 'j' + i
	}
	// Lowercase s-z
	for i := byte(0); i < 8; i++ {
		m[0xA2+i] = 's' + i
	}
	// Uppercase A-I
	for i := byte(0); i < 9; i++ {
		m[0xC1+i] = 'A' + i
	}
	// Uppercase J-R
	for i := byte(0); i < 9; i++ {
		m[0xD1+i] = 'J' + i
	}
	// Uppercase S-Z
	for i := byte(0); i < 8; i++ {
		m[0xE2+i] = 'S' + i
	}
	// Digits 0-9
	for i := byte(0); i < 10; i++ {
		m[0xF0+i] = '0' + i
	}
	return buildCodePage("cp1047", m)
}
