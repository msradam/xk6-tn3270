package tn3270

// EBCDIC Code Page 037 (US/Canada) conversion tables.

var ebcdicToASCII [256]byte
var asciiToEBCDIC [256]byte

func init() {
	// Default: unmapped EBCDIC codes display as space
	for i := range ebcdicToASCII {
		ebcdicToASCII[i] = ' '
	}
	ebcdicToASCII[0x00] = 0x00 // NUL

	// Space
	ebcdicToASCII[0x40] = ' '

	// Punctuation and special characters
	ebcdicToASCII[0x4B] = '.'
	ebcdicToASCII[0x4C] = '<'
	ebcdicToASCII[0x4D] = '('
	ebcdicToASCII[0x4E] = '+'
	ebcdicToASCII[0x4F] = '|'
	ebcdicToASCII[0x50] = '&'
	ebcdicToASCII[0x5A] = '!'
	ebcdicToASCII[0x5B] = '$'
	ebcdicToASCII[0x5C] = '*'
	ebcdicToASCII[0x5D] = ')'
	ebcdicToASCII[0x5E] = ';'
	ebcdicToASCII[0x5F] = '^'
	ebcdicToASCII[0x60] = '-'
	ebcdicToASCII[0x61] = '/'
	ebcdicToASCII[0x6B] = ','
	ebcdicToASCII[0x6C] = '%'
	ebcdicToASCII[0x6D] = '_'
	ebcdicToASCII[0x6E] = '>'
	ebcdicToASCII[0x6F] = '?'
	ebcdicToASCII[0x79] = '`'
	ebcdicToASCII[0x7A] = ':'
	ebcdicToASCII[0x7B] = '#'
	ebcdicToASCII[0x7C] = '@'
	ebcdicToASCII[0x7D] = '\''
	ebcdicToASCII[0x7E] = '='
	ebcdicToASCII[0x7F] = '"'
	ebcdicToASCII[0xA1] = '~'
	ebcdicToASCII[0xC0] = '{'
	ebcdicToASCII[0xD0] = '}'
	ebcdicToASCII[0xE0] = '\\'

	// Lowercase a-i (0x81-0x89)
	for i := byte(0); i < 9; i++ {
		ebcdicToASCII[0x81+i] = 'a' + i
	}
	// Lowercase j-r (0x91-0x99)
	for i := byte(0); i < 9; i++ {
		ebcdicToASCII[0x91+i] = 'j' + i
	}
	// Lowercase s-z (0xA2-0xA9)
	for i := byte(0); i < 8; i++ {
		ebcdicToASCII[0xA2+i] = 's' + i
	}

	// Uppercase A-I (0xC1-0xC9)
	for i := byte(0); i < 9; i++ {
		ebcdicToASCII[0xC1+i] = 'A' + i
	}
	// Uppercase J-R (0xD1-0xD9)
	for i := byte(0); i < 9; i++ {
		ebcdicToASCII[0xD1+i] = 'J' + i
	}
	// Uppercase S-Z (0xE2-0xE9)
	for i := byte(0); i < 8; i++ {
		ebcdicToASCII[0xE2+i] = 'S' + i
	}

	// Digits 0-9 (0xF0-0xF9)
	for i := byte(0); i < 10; i++ {
		ebcdicToASCII[0xF0+i] = '0' + i
	}

	// Build reverse mapping (ASCII → EBCDIC)
	for e := 0; e < 256; e++ {
		a := ebcdicToASCII[e]
		if a != ' ' && a != 0x00 && asciiToEBCDIC[a] == 0x00 {
			asciiToEBCDIC[a] = byte(e)
		}
	}
	asciiToEBCDIC[' '] = 0x40
	asciiToEBCDIC[0x00] = 0x00
}
