//go:build windows

package netcfg

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestDecodeConsoleGBKOnLink decodes the GBK bytes for 在链路上, the gateway
// column value `route print -4` prints for a directly connected route. Before
// DecodeConsole read the code pages from the registry, a build with
// activeCodePage=UTF-8 turned these bytes into ▯▯▯▯·▯▯.
func TestDecodeConsoleGBKOnLink(t *testing.T) {
	gbk := []byte{0xD4, 0xDA, 0xC1, 0xB4, 0xC2, 0xB7, 0xC9, 0xCF}
	if utf8.Valid(gbk) {
		t.Fatal("the GBK sample must not be valid UTF-8")
	}
	got := DecodeConsole(gbk)
	if got != "在链路上" {
		t.Fatalf("DecodeConsole = %q, want 在链路上 (code pages: %v)", got, consoleCodePages())
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Errorf("the decoded text still carries U+FFFD: %q", got)
	}

	// Output a program already wrote as UTF-8 is returned as is.
	if got := DecodeConsole([]byte("以太网 已连接")); got != "以太网 已连接" {
		t.Errorf("UTF-8 output changed: %q", got)
	}
	if got := DecodeConsole(nil); got != "" {
		t.Errorf("DecodeConsole(nil) = %q", got)
	}
}

func TestRunRoutePrintHasNoReplacementChar(t *testing.T) {
	out, err := Run(`route print -4`)
	if err != nil {
		t.Fatalf("Run(route print -4): %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("route print -4 returned empty output")
	}
	if strings.ContainsRune(out, utf8.RuneError) {
		t.Errorf("route print -4 output contains U+FFFD replacement char (code pages: %v)", consoleCodePages())
	}
}
