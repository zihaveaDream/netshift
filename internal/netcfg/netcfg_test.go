package netcfg

import (
	"strings"
	"testing"

	"netshift/internal/model"
)

func TestCommandsStatic(t *testing.T) {
	got, err := Commands(Request{
		Adapter:  "以太网",
		IPv4:     []string{"192.168.1.10"},
		Masks:    []string{"255.255.255.0"},
		Gateways: []model.Gateway{{Addr: "192.168.1.1"}},
		DNS:      []string{"192.168.1.1", "8.8.8.8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`netsh interface ip set address name="以太网" static 192.168.1.10 255.255.255.0 192.168.1.1 validate=no`,
		`netsh interface ip set dns name="以太网" static 192.168.1.1 validate=no primary`,
		`netsh interface ip add dns name="以太网" 8.8.8.8 index=2 validate=no`,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d commands: %v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("command %d:\n got %s\nwant %s", i, got[i], want[i])
		}
	}
}

func TestCommandsMultiAddressAndMetric(t *testing.T) {
	got, err := Commands(Request{
		Adapter:  "以太网",
		IPv4:     []string{"192.168.1.10", "10.0.0.5"},
		Masks:    []string{"255.255.255.0", "255.255.0.0"},
		Gateways: []model.Gateway{{Addr: "192.168.1.1", Metric: 1}, {Addr: "10.0.0.1", Metric: 5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got[0], "gwmetric=1") {
		t.Errorf("first command should carry the metric: %s", got[0])
	}
	found := false
	for _, c := range got {
		if strings.HasPrefix(c, `netsh interface ip add address name="以太网" 10.0.0.5 255.255.0.0 10.0.0.1 gwmetric=5`) {
			found = true
		}
	}
	if !found {
		t.Errorf("no add address for the second address: %v", got)
	}
	if got[len(got)-1] != `netsh interface ip set dns name="以太网" source=dhcp` {
		t.Errorf("DNS should be cleared when none is given: %s", got[len(got)-1])
	}
}

func TestCommandsDHCP(t *testing.T) {
	got, err := Commands(Request{Adapter: "以太网", DHCP: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`netsh interface ip set address name="以太网" source=dhcp validate=no`,
		`netsh interface ip set dns name="以太网" source=dhcp`,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("command %d = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestCommandsRejectBadInput(t *testing.T) {
	if _, err := Commands(Request{Adapter: `a"&calc`, DHCP: true}); err == nil {
		t.Error("adapter name with quotes accepted")
	}
	if _, err := Commands(Request{Adapter: "x", IPv4: []string{"1.2.3.999"}, Masks: []string{"255.255.255.0"}}); err == nil {
		t.Error("bad address accepted")
	}
	if _, err := Commands(Request{Adapter: "x", IPv4: []string{"1.2.3.4"}, Masks: []string{"255.0.255.0"}}); err == nil {
		t.Error("bad mask accepted")
	}
}

func TestSetEnabledAndMetric(t *testing.T) {
	line, err := SetEnabled("以太网", false)
	if err != nil {
		t.Fatal(err)
	}
	if line != `netsh interface set interface name="以太网" admin=disabled` {
		t.Errorf("disable = %s", line)
	}
	line, err = SetEnabled("以太网", true)
	if err != nil || !strings.HasSuffix(line, "admin=enabled") {
		t.Errorf("enable = %s, %v", line, err)
	}
	line, err = SetMetric("以太网", 20)
	if err != nil {
		t.Fatal(err)
	}
	if line != `netsh interface ip set interface interface="以太网" metric=20` {
		t.Errorf("metric = %s", line)
	}
	if _, err := SetMetric("以太网", 0); err == nil {
		t.Error("metric 0 accepted")
	}
}

func TestIPConfigCommands(t *testing.T) {
	if c, err := ReleaseCommand("以太网"); err != nil || c != `ipconfig /release "以太网"` {
		t.Errorf("release = %s, %v", c, err)
	}
	if c, err := RenewCommand("以太网"); err != nil || c != `ipconfig /renew "以太网"` {
		t.Errorf("renew = %s, %v", c, err)
	}
	if c := FlushDNSCommand(); c != `ipconfig /flushdns` {
		t.Errorf("flush = %s", c)
	}
}

func TestAdapterLabels(t *testing.T) {
	a := Adapter{Name: "以太网", IPv4: []string{"192.168.1.10"}, Up: true}
	if a.Label() != "以太网 (192.168.1.10)" {
		t.Errorf("label = %s", a.Label())
	}
	if a.Status() != "已连接" {
		t.Errorf("status = %s", a.Status())
	}
	a.Up = false
	if a.Status() != "已断开" {
		t.Errorf("status = %s", a.Status())
	}
}

func TestExtraGatewaysAndMultiGateways(t *testing.T) {
	got, err := AddressCommands(Request{
		Adapter: "以太网",
		IPv4:    []string{"192.168.1.10"},
		Masks:   []string{"255.255.255.0"},
		Gateways: []model.Gateway{
			{Addr: "192.168.1.1", Metric: 1},
			{Addr: "192.168.1.254", Metric: 10},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 commands, got %v", got)
	}
	if !strings.Contains(got[1], `gateway=192.168.1.254 gwmetric=10`) {
		t.Errorf("extra gateway command missing: %s", got[1])
	}

	gcmds, err := SetGatewaysCommands("以太网", []model.Gateway{
		{Addr: "192.168.1.1"},
		{Addr: "192.168.1.2", Metric: 5},
	})
	if err != nil || len(gcmds) != 2 {
		t.Fatalf("SetGatewaysCommands = %v, %v", gcmds, err)
	}
}
