//go:build windows

package netcfg

import (
	"os"
	"testing"
)

// TestLiveAdapters prints what the machine reports. It is skipped unless
// NETSHIFT_LIVE=1, so `go test ./...` stays offline and read-only; run it with
//
//	$env:NETSHIFT_LIVE='1'; go test -run TestLiveAdapters -v ./internal/netcfg
func TestLiveAdapters(t *testing.T) {
	if os.Getenv("NETSHIFT_LIVE") != "1" {
		t.Skip("set NETSHIFT_LIVE=1 to read the machine's adapters")
	}
	list, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, a := range list {
		t.Logf("name=%q desc=%q guid=%s index=%d up=%v dhcp=%v mac=%s",
			a.Name, a.Description, a.GUID, a.Index, a.Up, a.DHCP, a.MAC)
		t.Logf("    IPv4=%v Masks=%v Gateways=%v DNS=%v label=%q",
			a.IPv4, a.Masks, a.Gateways, a.DNS, a.Label())
		if a.Index <= 0 {
			t.Errorf("%s has no interface index", a.Name)
		}
	}
	if len(list) == 0 {
		t.Skip("this machine reports no adapters (offline VM?)")
	}
}
