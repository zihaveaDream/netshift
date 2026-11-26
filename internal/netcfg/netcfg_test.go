package netcfg

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
		`netsh interface ip set address name="以太网" static 192.168.1.10 255.255.255.0 192.168.1.1`,
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
		`netsh interface ip set address name="以太网" source=dhcp`,
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
	for _, c := range gcmds {
		if strings.Contains(c, "validate=") {
			t.Errorf("set/add address gateway command must not carry validate=: %s", c)
		}
	}
}

func TestAddressCommandsNeverUseValidate(t *testing.T) {
	cases := []Request{
		{Adapter: "以太网", DHCP: true},
		{Adapter: "以太网", IPv4: []string{"192.168.1.10"}, Masks: []string{"255.255.255.0"}},
		{Adapter: "以太网", IPv4: []string{"192.168.1.10", "10.0.0.2"}, Masks: []string{"255.255.255.0", "255.0.0.0"},
			Gateways: []model.Gateway{{Addr: "192.168.1.1", Metric: 1}, {Addr: "10.0.0.1"}}},
	}
	for _, r := range cases {
		cmds, err := AddressCommands(r)
		if err != nil {
			t.Fatalf("AddressCommands(%+v): %v", r, err)
		}
		for _, c := range cmds {
			if strings.Contains(c, "validate=") {
				t.Errorf("address command must not carry validate=: %s", c)
			}
		}
	}
}

func TestCheckNetshResult(t *testing.T) {
	// Syntax error with exit code 0 (English and Chinese) must be caught.
	err := checkNetshResult(
		`netsh interface ip set address name="以太网" source=dhcp validate=no`,
		"'validate' is not a valid argument for this command.\nUsage: set address ...",
		nil,
	)
	if err == nil {
		t.Error("expected syntax error on English netsh output with exit 0")
	}
	err = checkNetshResult(
		`netsh interface ip set address name="以太网" source=dhcp validate=no`,
		"'validate' 不是此命令的有效参数。\n用法: set address ...",
		nil,
	)
	if err == nil {
		t.Error("expected syntax error on Chinese netsh output with exit 0")
	}

	// Idempotent "DHCP is already enabled" with non-zero exit must be treated as success.
	runErr := errors.New("exit status 1")
	if err := checkNetshResult(
		`netsh interface ip set address name="以太网" source=dhcp`,
		"DHCP is already enabled on this interface.",
		runErr,
	); err != nil {
		t.Errorf("English already-DHCP should succeed: %v", err)
	}
	if err := checkNetshResult(
		`netsh interface ip set dns name="以太网" source=dhcp`,
		"此接口上已启用 DHCP。",
		runErr,
	); err != nil {
		t.Errorf("Chinese already-DHCP should succeed: %v", err)
	}

	// Real failure on non-DHCP or other error must still propagate.
	if err := checkNetshResult(
		`netsh interface ip set address name="以太网" source=dhcp`,
		"The filename, directory name, or volume label syntax is incorrect.",
		runErr,
	); err == nil {
		t.Error("real netsh error should not be swallowed")
	}

	// Normal success output passes through cleanly.
	if err := checkNetshResult(`netsh interface ip set address name="以太网" source=dhcp`, "", nil); err != nil {
		t.Errorf("clean success rejected: %v", err)
	}
}

func TestIsAPIPAAndUsableIPv4(t *testing.T) {
	for _, ip := range []string{"169.254.10.20", " 169.254.0.1 ", "169.254.255.254"} {
		if !IsAPIPA(ip) {
			t.Errorf("IsAPIPA(%q) = false, want true", ip)
		}
	}
	for _, ip := range []string{"192.168.1.10", "10.0.0.1", "169.253.1.1", "0.0.0.0", ""} {
		if IsAPIPA(ip) {
			t.Errorf("IsAPIPA(%q) = true, want false", ip)
		}
	}

	apipa := Adapter{IPv4: []string{"169.254.7.7"}}
	if apipa.UsableIPv4() != "" || apipa.HasUsableIPv4() {
		t.Errorf("an APIPA address is not usable: %q", apipa.UsableIPv4())
	}
	empty := Adapter{}
	if empty.UsableIPv4() != "" || empty.HasUsableIPv4() {
		t.Error("an adapter without addresses has no usable IPv4")
	}
	normal := Adapter{IPv4: []string{"192.168.1.10"}}
	if normal.UsableIPv4() != "192.168.1.10" || !normal.HasUsableIPv4() {
		t.Errorf("UsableIPv4 = %q", normal.UsableIPv4())
	}
	// The automatic address Windows keeps next to the real one must be skipped.
	mixed := Adapter{IPv4: []string{"169.254.7.7", "192.168.1.10", "10.0.0.5"}}
	if mixed.UsableIPv4() != "192.168.1.10" || !mixed.HasUsableIPv4() {
		t.Errorf("UsableIPv4 = %q, want 192.168.1.10", mixed.UsableIPv4())
	}
}

func TestDecodeWithCandidatesSkipsUTF8AndFallbacks(t *testing.T) {
	// 在链路上 encoded as GBK, the way cmd.exe prints it on a Chinese Windows.
	gbk := []byte{0xD4, 0xDA, 0xC1, 0xB4, 0xC2, 0xB7, 0xC9, 0xCF}

	// OEMCP/ACP report 65001 in a build whose manifest sets activeCodePage UTF-8;
	// 0 means unknown. Both must be dropped, 936 must survive once.
	cps := codePageCandidates([]uint32{65001, 0, 936, 936, 65001}, 936, 54936)
	want := []uint32{936, 54936}
	if len(cps) != len(want) {
		t.Fatalf("candidates = %v, want %v", cps, want)
	}
	for i := range want {
		if cps[i] != want[i] {
			t.Fatalf("candidate %d = %d, want %d", i, cps[i], want[i])
		}
	}

	var called []uint32
	convert := func(cp, flags uint32, data []byte) (string, bool) {
		called = append(called, cp)
		if cp != 936 {
			return "", false
		}
		if string(data) != string(gbk) {
			return "", false
		}
		return "在链路上", true
	}
	if got := decodeWithCandidates(gbk, cps, convert); got != "在链路上" {
		t.Errorf("decodeWithCandidates = %q, want 在链路上", got)
	}
	for _, cp := range called {
		if cp == 65001 {
			t.Error("65001 must never be tried: the bytes are not UTF-8")
		}
	}

	// Valid UTF-8 passes through untouched, without consulting any code page.
	called = nil
	if got := decodeWithCandidates([]byte("以太网"), cps, convert); got != "以太网" {
		t.Errorf("UTF-8 input changed: %q", got)
	}
	if len(called) != 0 {
		t.Errorf("UTF-8 input should not reach the code pages: %v", called)
	}

	// A UTF-8 BOM is stripped, and an empty sample stays empty.
	if got := decodeWithCandidates(append([]byte{0xEF, 0xBB, 0xBF}, []byte("以太网")...), cps, convert); got != "以太网" {
		t.Errorf("BOM was not stripped: %q", got)
	}
	if got := decodeWithCandidates(nil, cps, convert); got != "" {
		t.Errorf("empty input = %q", got)
	}
	if got := decodeWithCandidates([]byte{0xEF, 0xBB, 0xBF}, cps, convert); got != "" {
		t.Errorf("BOM-only input = %q", got)
	}

	// Nothing converts: the last resort repairs the bytes instead of keeping them.
	if got := decodeWithCandidates([]byte{0xFF, 0xFE, 'A'}, []uint32{936}, func(uint32, uint32, []byte) (string, bool) {
		return "", false
	}); got != "?A" {
		t.Errorf("lossy fallback = %q, want ?A", got)
	}
}

func TestDecodeWithCandidatesLenientRound(t *testing.T) {
	gbk := []byte{0xD4, 0xDA, 0xC1, 0xB4, 0xC2, 0xB7, 0xC9, 0xCF}
	var flags []uint32
	// 936 supports MB_ERR_INVALID_CHARS: the strict round fails, the lenient one wins.
	convert := func(cp, f uint32, data []byte) (string, bool) {
		flags = append(flags, f)
		if f&mbErrInvalidChars != 0 {
			return "", false
		}
		return "宽松兜底", true
	}
	if got := decodeWithCandidates(gbk, []uint32{936}, convert); got != "宽松兜底" {
		t.Errorf("lenient round = %q, want 宽松兜底", got)
	}
	if len(flags) != 2 || flags[0] != mbErrInvalidChars || flags[1] != 0 {
		t.Errorf("flags = %v, want [%#x 0]", flags, mbErrInvalidChars)
	}

	// 54936 cannot take the strict flag, so the first round already runs leniently.
	flags = nil
	if got := decodeWithCandidates(gbk, []uint32{54936}, convert); got != "宽松兜底" {
		t.Errorf("54936 round = %q, want 宽松兜底", got)
	}
	if len(flags) != 1 || flags[0] != 0 {
		t.Errorf("54936 flags = %v, want [0]", flags)
	}
}

func TestAwaitAdapterWith(t *testing.T) {
	listWith := func(ad Adapter) func() ([]Adapter, error) {
		return func() ([]Adapter, error) { return []Adapter{ad}, nil }
	}
	usable := func(ad Adapter) bool { return ad.HasUsableIPv4() }

	// Already satisfied: one sample, no wait.
	samples := 0
	list, ad, ok := awaitAdapterWith(context.Background(), "以太网", 10*time.Millisecond,
		func() ([]Adapter, error) {
			samples++
			return listWith(Adapter{Name: "以太网", Up: true, DHCP: true, IPv4: []string{"192.168.1.10"}})()
		}, usable, nil)
	if !ok || ad.UsableIPv4() != "192.168.1.10" || len(list) != 1 {
		t.Fatalf("immediate match = %+v, %+v, %v", list, ad, ok)
	}
	if samples != 1 {
		t.Errorf("sampled %d times, want 1", samples)
	}

	// Satisfied on the third round; onUpdate reports each state change once.
	round := 0
	var updates []string
	list, ad, ok = awaitAdapterWith(context.Background(), "以太网", 5*time.Millisecond,
		func() ([]Adapter, error) {
			round++
			ip := ""
			switch {
			case round == 3:
				ip = "192.168.1.20"
			case round > 3:
				ip = "10.0.0.9"
			}
			return listWith(Adapter{Name: "以太网", Up: true, DHCP: true, IPv4: []string{ip}})()
		}, usable, func(_ []Adapter, ad Adapter) { updates = append(updates, ad.CurrentIP()) })
	if !ok || ad.UsableIPv4() != "192.168.1.20" || len(list) != 1 {
		t.Fatalf("third-round match = %+v, %+v, %v", list, ad, ok)
	}
	if round != 3 {
		t.Errorf("polled %d rounds, want 3", round)
	}
	if len(updates) != 1 || updates[0] != "" {
		t.Errorf("updates = %v, want one change from no IP", updates)
	}

	// Timeout with an APIPA address still returns the last sample.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	list, ad, ok = awaitAdapterWith(ctx, "以太网", 5*time.Millisecond,
		listWith(Adapter{Name: "以太网", Up: true, DHCP: true, IPv4: []string{"169.254.3.3"}}), usable, nil)
	if ok {
		t.Error("an APIPA address must not satisfy the wait")
	}
	if ad.CurrentIP() != "169.254.3.3" || len(list) != 1 {
		t.Errorf("timeout sample = %+v, %+v", list, ad)
	}

	// Cancelling returns immediately, without waiting for the next poll.
	ctx2, cancel2 := context.WithCancel(context.Background())
	calls := 0
	done := make(chan bool, 1)
	go func() {
		_, _, ok := awaitAdapterWith(ctx2, "以太网", time.Second,
			func() ([]Adapter, error) {
				calls++
				return listWith(Adapter{Name: "以太网", Up: true, DHCP: true})()
			}, usable, nil)
		done <- ok
	}()
	time.Sleep(30 * time.Millisecond)
	cancel2()
	select {
	case ok := <-done:
		if ok {
			t.Error("a cancelled wait must not report success")
		}
	case <-time.After(time.Second):
		t.Fatal("the wait kept running after it was cancelled")
	}
	if calls != 1 {
		t.Errorf("polled %d times while waiting for a 1s tick, want 1", calls)
	}

	// A failing list is retried instead of ending the wait.
	attempt := 0
	_, ad, ok = awaitAdapterWith(context.Background(), "以太网", 5*time.Millisecond,
		func() ([]Adapter, error) {
			attempt++
			if attempt == 1 {
				return nil, errors.New("GetAdaptersAddresses failed")
			}
			return listWith(Adapter{Name: "以太网", Up: true, DHCP: true, IPv4: []string{"192.168.1.30"}})()
		}, usable, nil)
	if !ok || ad.UsableIPv4() != "192.168.1.30" {
		t.Errorf("recovery after a list error = %+v, %v", ad, ok)
	}
}
