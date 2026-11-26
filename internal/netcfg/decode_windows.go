//go:build windows

package netcfg

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// consoleCodePages caches the code pages console output may be encoded in: the
// registry is read once instead of on every command.
var (
	consoleCPsOnce sync.Once
	consoleCPs     []uint32
)

// DecodeConsole turns the bytes a command wrote to its console into UTF-8.
//
// A process whose manifest sets <activeCodePage>UTF-8</activeCodePage> (mygo
// does that when it packages this tool) reports 65001 to GetOEMCP/GetACP, while
// cmd.exe, netsh and route keep writing the system's OEM code page (936 on a
// Chinese Windows). Reading the code pages from the registry keeps the decode
// correct in both cases.
func DecodeConsole(b []byte) string {
	return decodeWithCandidates(b, consoleCodePages(), multiByteToUTF8)
}

// multiByteToUTF8 is the MultiByteToWideChar conversion the decoder tries; ok is
// false when the code page cannot convert data with the given flags.
func multiByteToUTF8(cp, flags uint32, data []byte) (string, bool) {
	if len(data) == 0 {
		return "", true
	}
	n, err := windows.MultiByteToWideChar(cp, flags, &data[0], int32(len(data)), nil, 0)
	if err != nil || n <= 0 {
		return "", false
	}
	wide := make([]uint16, n)
	if _, err := windows.MultiByteToWideChar(cp, flags, &data[0], int32(len(data)), &wide[0], n); err != nil {
		return "", false
	}
	return strings.TrimRight(string(utf16.Decode(wide)), "\x00"), true
}

// consoleCodePages returns the code pages to try, in order of likelihood: the
// system's OEM and ANSI code pages, what the process reports, then the CJK
// fallbacks.
func consoleCodePages() []uint32 {
	consoleCPsOnce.Do(func() {
		raw := []uint32{
			registryCodePage("OEMCP"),
			registryCodePage("ACP"),
			kernelCodePage("GetConsoleOutputCP"),
			kernelCodePage("GetOEMCP"),
			kernelCodePage("GetACP"),
		}
		consoleCPs = codePageCandidates(raw, 936, 54936)
	})
	return consoleCPs
}

// registryCodePage reads a code page number from the NLS CodePage key, the one
// place an <activeCodePage> manifest cannot rewrite. It returns 0 when the value
// is missing.
func registryCodePage(name string) uint32 {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Nls\CodePage`, registry.QUERY_VALUE)
	if err != nil {
		return 0
	}
	defer key.Close()
	v, _, err := key.GetStringValue(name)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}

// kernelCodePage calls a kernel32 code page getter and returns 0 on failure.
func kernelCodePage(name string) uint32 {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc(name)
	if err := proc.Find(); err != nil {
		return 0
	}
	v, _, _ := proc.Call()
	return uint32(v)
}
