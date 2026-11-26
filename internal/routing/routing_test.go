package routing

import (
	"strings"
	"testing"
	"unicode/utf8"

	"netshift/internal/netcfg"
)

func TestCandidatesStartWithOwnNetworks(t *testing.T) {
	got := Candidates("192.168.1.10")
	if len(got) != 5 {
		t.Fatalf("got %d candidates: %+v", len(got), got)
	}
	want := []struct{ net, mask string }{
		{"192.168.0.0", "255.255.0.0"},
		{"192.168.1.0", "255.255.255.0"},
		{"192.0.0.0", "255.0.0.0"},
		{"172.16.0.0", "255.240.0.0"},
		{"10.0.0.0", "255.0.0.0"},
	}
	for i, w := range want {
		if i >= len(got) {
			break
		}
		if got[i].Net != w.net || got[i].Mask != w.mask {
			t.Errorf("candidate %d = %s/%s, want %s/%s", i, got[i].Net, got[i].Mask, w.net, w.mask)
		}
	}
	// The duplicate of 192.168.0.0/16 appears once.
	count := 0
	for _, c := range got {
		if c.Net == "192.168.0.0" && c.Mask == "255.255.0.0" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("192.168.0.0/16 listed %d times", count)
	}
}

func TestIsDirect(t *testing.T) {
	if !IsDirect("192.168.1.10", "255.255.255.0", "192.168.1.0", "255.255.255.0") {
		t.Error("own network should be direct")
	}
	if IsDirect("192.168.1.10", "255.255.255.0", "10.0.0.0", "255.0.0.0") {
		t.Error("another network should not be direct")
	}
	if IsDirect("192.168.1.10", "255.255.255.0", "192.168.1.0", "255.255.0.0") {
		t.Error("a different mask should not be direct")
	}
}

func TestCommands(t *testing.T) {
	p := Plan{Adapter: "以太网", IfIndex: 12, Net: "10.0.0.0", Mask: "255.0.0.0", Gateway: "192.168.1.1"}
	got, err := p.Commands()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if got[0] != `route delete 0.0.0.0 mask 0.0.0.0 192.168.1.1` {
		t.Errorf("delete = %s", got[0])
	}
	if got[1] != `route add 10.0.0.0 mask 255.0.0.0 192.168.1.1 if 12` {
		t.Errorf("add = %s", got[1])
	}
}

func TestCommandsPersist(t *testing.T) {
	p := Plan{Adapter: "以太网", IfIndex: 12, Net: "10.0.0.0", Mask: "255.0.0.0", Gateway: "192.168.1.1", Persist: true}
	got, err := p.Commands()
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != `route add 10.0.0.0 mask 255.0.0.0 192.168.1.1 if 12 -p` {
		t.Errorf("persistent add = %s", got[1])
	}
}

func TestCommandsDirect(t *testing.T) {
	p := Plan{Adapter: "以太网", IfIndex: 12, Net: "192.168.1.0", Mask: "255.255.255.0",
		Gateway: "192.168.1.1", Direct: true}
	got, err := p.Commands()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "route delete") {
		t.Errorf("direct plan should only delete the default route: %v", got)
	}
}

func TestCommandsKeepDefault(t *testing.T) {
	p := Plan{Adapter: "以太网", Net: "10.0.0.0", Mask: "255.0.0.0", Gateway: "192.168.1.1", KeepDefault: true}
	got, err := p.Commands()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "route add") {
		t.Errorf("keeping the default route should only add: %v", got)
	}
}

func TestPlanValidation(t *testing.T) {
	bad := []Plan{
		{Adapter: "以太网", Net: "", Mask: "255.0.0.0", Gateway: "1.1.1.1"},
		{Adapter: "以太网", Net: "10.0.0.0", Mask: "255.0.255.0", Gateway: "1.1.1.1"},
		{Adapter: "以太网", Net: "10.0.0.0", Mask: "255.0.0.0", Gateway: ""},
		{Adapter: `a"b`, Net: "10.0.0.0", Mask: "255.0.0.0", Gateway: "1.1.1.1"},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Errorf("plan %d should be invalid: %+v", i, p)
		}
	}
	good := Plan{Adapter: "以太网", Net: "10.0.0.0", Mask: "255.0.0.0", Gateway: "1.1.1.1"}
	if err := good.Validate(); err != nil {
		t.Errorf("valid plan rejected: %v", err)
	}
}

func TestRouteExists(t *testing.T) {
	table := `
          0.0.0.0          0.0.0.0      192.168.1.1     192.168.1.10     25
       10.0.0.0        255.0.0.0      192.168.1.1     192.168.1.10     26
`
	if !RouteExists(table, "10.0.0.0", "192.168.1.1") {
		t.Error("route not found")
	}
	if RouteExists(table, "10.0.0.0", "10.9.9.9") {
		t.Error("route found through the wrong gateway")
	}
}

func TestGatewayFor(t *testing.T) {
	if got := GatewayFor([]string{"192.168.1.1"}, "10.0.0.1", "10.0.0.0"); got != "192.168.1.1" {
		t.Errorf("adapter gateway should win: %s", got)
	}
	if got := GatewayFor(nil, "10.0.0.1", "10.0.0.0"); got != "10.0.0.1" {
		t.Errorf("saved gateway should be used: %s", got)
	}
	if got := GatewayFor(nil, "", "10.7.0.0"); got != "10.7.0.1" {
		t.Errorf("fallback should be the network's .1: %s", got)
	}
}

func TestParseInterfaceIndexSafeOnShortLines(t *testing.T) {
	sample := "Idx     Met\n---\n  12    25    1500  connected     以太网\n  15    50    1500  connected     WLAN 2\n"
	if idx := parseInterfaceIndex(sample, "以太网"); idx != 12 {
		t.Errorf("以太网 idx = %d, want 12", idx)
	}
	if idx := parseInterfaceIndex(sample, "WLAN 2"); idx != 15 {
		t.Errorf("WLAN 2 idx = %d, want 15", idx)
	}
}

func TestParseTableAndDeleteCommand(t *testing.T) {
	sample := `
活动路由:
网络目标        网络掩码          网关       接口   跃点数
          0.0.0.0          0.0.0.0      192.168.1.1     192.168.1.10     25
       10.0.0.0        255.0.0.0      192.168.1.1     192.168.1.10     26
    192.168.1.0    255.255.255.0            在链路上     192.168.1.10    281
永久路由:
  网络地址          网络掩码  网关地址  跃点数
     172.16.0.0      255.240.0.0      192.168.1.1       1
`
	entries := ParseTable(sample)
	if len(entries) != 4 {
		t.Fatalf("parsed %d routes, want 4: %+v", len(entries), entries)
	}
	if entries[1].Dest != "10.0.0.0" || entries[1].Persistent {
		t.Errorf("active route 1 = %+v", entries[1])
	}
	if !entries[3].Persistent || entries[3].Dest != "172.16.0.0" || entries[3].Gateway != "192.168.1.1" {
		t.Errorf("persistent route = %+v", entries[3])
	}
	cmd, err := DeleteCommand(entries[3])
	if err != nil || cmd != "route delete 172.16.0.0 mask 255.240.0.0 192.168.1.1 -p" {
		t.Errorf("DeleteCommand(persistent) = %q, %v", cmd, err)
	}
	cmd2, err := DeleteCommand(entries[2])
	if err != nil || cmd2 != "route delete 192.168.1.0 mask 255.255.255.0" {
		t.Errorf("DeleteCommand(on-link) = %q, %v", cmd2, err)
	}
}

// TestParseTableAfterConsoleDecode feeds ParseTable what `route print -4` prints
// on a Chinese Windows: GBK bytes decoded through netcfg.DecodeConsole. The
// on-link gateway must come out as 在链路上 instead of U+FFFD garbage.
func TestParseTableAfterConsoleDecode(t *testing.T) {
	var gbk []byte
	add := func(s string) { gbk = append(gbk, []byte(s)...) }
	// 活动路由:
	gbk = append(gbk, 0xBB, 0xEE, 0xB6, 0xAF, 0xC2, 0xB7, 0xD3, 0xC9)
	add(":\r\n")
	add("          0.0.0.0          0.0.0.0      192.168.1.1     192.168.1.10     25\r\n")
	add("    192.168.1.0    255.255.255.0   ")
	// 在链路上
	gbk = append(gbk, 0xD4, 0xDA, 0xC1, 0xB4, 0xC2, 0xB7, 0xC9, 0xCF)
	add("     192.168.1.10    281\r\n")
	// 永久路由:
	gbk = append(gbk, 0xD3, 0xC0, 0xBE, 0xC3, 0xC2, 0xB7, 0xD3, 0xC9)
	add(":\r\n")
	add("     172.16.0.0      255.240.0.0      192.168.1.1       1\r\n")

	text := netcfg.DecodeConsole(gbk)
	if strings.ContainsRune(text, utf8.RuneError) {
		t.Fatalf("decoded route table still carries U+FFFD:\n%s", text)
	}
	entries := ParseTable(text)
	if len(entries) != 3 {
		t.Fatalf("parsed %d routes, want 3:\n%s", len(entries), text)
	}
	if entries[1].Dest != "192.168.1.0" || entries[1].Gateway != "在链路上" {
		t.Errorf("on-link route = %+v, want gateway 在链路上", entries[1])
	}
	if !entries[2].Persistent || entries[2].Dest != "172.16.0.0" || entries[2].Gateway != "192.168.1.1" {
		t.Errorf("persistent route = %+v", entries[2])
	}
}
