//go:build !windows

package netcfg

import "strings"

// DecodeConsole returns b as UTF-8 on the platforms whose tools already write
// UTF-8, replacing whatever is not valid.
func DecodeConsole(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return strings.ToValidUTF8(string(b), "?")
}
