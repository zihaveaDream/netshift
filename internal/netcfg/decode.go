package netcfg

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// utf8BOM is the byte order mark Windows tools sometimes prepend to output.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// mbErrInvalidChars is MultiByteToWideChar's MB_ERR_INVALID_CHARS flag: make the
// conversion fail instead of substituting U+FFFD for bytes the code page has no
// mapping for.
const mbErrInvalidChars = 0x00000008

// codePageCandidates merges the code pages command output may be encoded in,
// dropping duplicates, 0 (unknown) and 65001: UTF-8 is recognized on its own by
// utf8.Valid, so 65001 would only ever be tried on bytes that are not UTF-8.
// raw holds the code pages as the system reports them, fallbacks the ones to try
// when the system reports nothing usable, in order.
func codePageCandidates(raw []uint32, fallbacks ...uint32) []uint32 {
	var out []uint32
	seen := make(map[uint32]bool, len(raw)+len(fallbacks))
	for _, group := range [][]uint32{raw, fallbacks} {
		for _, cp := range group {
			if cp == 0 || cp == 65001 || seen[cp] {
				continue
			}
			seen[cp] = true
			out = append(out, cp)
		}
	}
	return out
}

// decodeWithCandidates turns console output copied from a Windows command into
// UTF-8 by trying the code pages it may be encoded in. Output that a program
// wrote as UTF-8 (or with a UTF-8 BOM) is returned untouched; otherwise the
// first code page that maps every byte cleanly wins, with a lenient retry and a
// final lossy pass as fallbacks. convert is MultiByteToWideChar on Windows and
// a fake in tests.
func decodeWithCandidates(b []byte, cps []uint32, convert func(cp, flags uint32, data []byte) (string, bool)) string {
	if len(b) == 0 {
		return ""
	}
	if bytes.HasPrefix(b, utf8BOM) {
		b = b[len(utf8BOM):]
		if len(b) == 0 {
			return ""
		}
	}
	if utf8.Valid(b) {
		return string(b)
	}
	if convert != nil {
		for _, cp := range cps {
			flags := uint32(mbErrInvalidChars)
			if !supportsStrictMB(cp) {
				flags = 0
			}
			if s, ok := convert(cp, flags, b); ok && !strings.ContainsRune(s, utf8.RuneError) {
				return s
			}
		}
		// Second, lenient round: let the most likely code page replace what it
		// cannot map, which still beats decoding the bytes as UTF-8.
		if len(cps) > 0 {
			if s, ok := convert(cps[0], 0, b); ok {
				return s
			}
		}
	}
	return strings.ToValidUTF8(string(b), "?")
}

// supportsStrictMB reports whether a code page accepts MB_ERR_INVALID_CHARS,
// the flag the strict round relies on; the others are only tried leniently.
func supportsStrictMB(cp uint32) bool {
	switch {
	case cp == 54936, cp == 52936, cp == 65000, cp == 65001:
		return false
	case cp >= 50220 && cp <= 50229: // ISO-2022-JP and its siblings
		return false
	}
	return true
}
