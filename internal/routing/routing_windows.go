//go:build windows

package routing

import (
	"fmt"
	"strconv"
	"strings"

	"netshift/internal/netcfg"
)

// Apply runs the plan's commands, one at a time, and returns what ran plus the
// output of each command. It stops at the first failure.
func Apply(p Plan) (ran []string, output string, err error) {
	lines, cerr := p.Commands()
	if cerr != nil {
		return nil, "", cerr
	}
	var b strings.Builder
	for _, line := range lines {
		ran = append(ran, line)
		out, rerr := netcfg.Run(line)
		if out != "" {
			b.WriteString("> " + line + "\n" + out + "\n")
		}
		if rerr != nil {
			// A missing default route to delete is not a failure.
			if strings.HasPrefix(line, "route delete") {
				continue
			}
			return ran, b.String(), fmt.Errorf("%s 失败：%w", line, rerr)
		}
	}
	return ran, b.String(), nil
}

// Table returns route print -4 output.
func Table() (string, error) {
	return netcfg.Run(`route print -4`)
}

// Delete removes the given route entry from the Windows routing table.
func Delete(e RouteEntry) (string, error) {
	cmd, err := DeleteCommand(e)
	if err != nil {
		return "", err
	}
	return netcfg.Run(cmd)
}

// InterfaceIndex returns the IPv4 interface index of an adapter through
// netsh, as a fallback when the adapter list has none.
func InterfaceIndex(adapter string) int {
	out, err := netcfg.Run(`netsh interface ipv4 show interfaces`)
	if err != nil {
		return 0
	}
	return parseInterfaceIndex(out, adapter)
}

func parseInterfaceIndex(out, adapter string) int {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		name := strings.Join(f[4:], " ")
		if name == adapter {
			if n, err := strconv.Atoi(f[0]); err == nil {
				return n
			}
		}
	}
	return 0
}
