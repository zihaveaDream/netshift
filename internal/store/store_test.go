package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"netshift/internal/model"
)

func testProfile(name string) model.Profile {
	return model.Profile{
		Name:      name,
		Location:  "北京-XX",
		Adapter:   "以太网",
		IPv4:      []string{"192.168.1.10", "10.0.0.5"},
		Masks:     []string{"255.255.255.0", "255.0.0.0"},
		Gateways:  []model.Gateway{{Addr: "192.168.1.1", Metric: 1}, {Addr: "10.0.0.1"}},
		DNS:       []string{"192.168.1.1", "8.8.8.8"},
		Computer:  "PC-0001",
		Workgroup: "WORKGROUP",
		MAC:       "AABBCCDDEEFF",
		Usage:     3,
		UpdatedAt: time.Date(2026, 11, 25, 10, 0, 0, 0, time.Local),
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	in := []model.Profile{testProfile("办公室"), {Name: "家里", DHCP: true, DNS: []string{"1.1.1.1"}}}
	if err := s.Save(in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d profiles, want 2", len(got))
	}
	byName := map[string]model.Profile{}
	for _, p := range got {
		byName[p.Name] = p
	}
	want := in[0]
	gotP := byName["办公室"]
	if gotP.Location != want.Location || gotP.Adapter != want.Adapter {
		t.Errorf("scalar fields differ: %+v", gotP)
	}
	if strings.Join(gotP.IPv4, ",") != strings.Join(want.IPv4, ",") {
		t.Errorf("IPv4 = %v, want %v", gotP.IPv4, want.IPv4)
	}
	if len(gotP.Gateways) != 2 || gotP.Gateways[0].Metric != 1 || gotP.Gateways[1].Addr != "10.0.0.1" {
		t.Errorf("gateways = %+v", gotP.Gateways)
	}
	if strings.Join(gotP.DNS, ",") != "192.168.1.1,8.8.8.8" {
		t.Errorf("DNS = %v", gotP.DNS)
	}
	if !byName["家里"].DHCP {
		t.Error("DHCP flag lost")
	}
	if !gotP.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", gotP.UpdatedAt, want.UpdatedAt)
	}
}

func TestLoadMissingFile(t *testing.T) {
	s := New(t.TempDir())
	got, err := s.Load()
	if err != nil || len(got) != 0 {
		t.Fatalf("Load() = %v, %v", got, err)
	}
}

func TestLoadBadFileIsBackedUp(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := os.WriteFile(filepath.Join(dir, ProfileFile), []byte("[方案\nIP=1.2.3.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Fatal("expected a parse error")
	}
	entries, _ := os.ReadDir(dir)
	backedUp := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "ip.dat.bak-") {
			backedUp = true
		}
	}
	if !backedUp {
		t.Error("the unreadable file was not backed up")
	}
}

func TestImportLegacy(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	content := "北京-XX,办公,以太网,192.168.1.10,255.255.255.0,192.168.1.1,192.168.1.1,8.8.8.8\r\n" +
		"上海-YY,测试,以太网,10.1.1.5,255.0.0.0,未填写,未填写,未填写\r\n" +
		"旧格式,以太网,172.16.1.5,255.255.0.0,172.16.0.1,1.1.1.1,2.2.2.2\r\n"
	if err := os.WriteFile(filepath.Join(dir, LegacyFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.ImportLegacy()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("imported %d profiles, want 3", len(got))
	}
	if got[0].Name != "北京-XX 办公" || got[0].Location != "北京-XX" || got[0].PrimaryIP() != "192.168.1.10" {
		t.Errorf("first profile = %+v", got[0])
	}
	if got[0].PrimaryGateway() != "192.168.1.1" || got[0].SecondaryDNS() != "8.8.8.8" {
		t.Errorf("first profile addresses = %+v", got[0])
	}
	if got[1].PrimaryGateway() != "" || len(got[1].DNS) != 0 {
		t.Errorf("未填写 should import as empty: %+v", got[1])
	}
	if got[2].PrimaryIP() != "172.16.1.5" {
		t.Errorf("7-field line = %+v", got[2])
	}
}

func TestLogFormat(t *testing.T) {
	p := testProfile("办公室")
	text := LogFormat(time.Date(2026, 11, 25, 10, 20, 1, 0, time.Local), "北京-XX", "办公", "以太网", p)
	for _, want := range []string{"时间：2026-11-25 10:20:01", "项目地点：北京-XX", "网卡：以太网", "IP地址：192.168.1.10 10.0.0.5", "备用 DNS：8.8.8.8"} {
		if !strings.Contains(text, want) {
			t.Errorf("log missing %q:\n%s", want, text)
		}
	}
}

func TestAppendLog(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := s.AppendLog("a\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendLog("b\n"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, LogFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "a\nb\n" {
		t.Errorf("log = %q", raw)
	}
	recent, err := s.ReadRecentLog(1)
	if err != nil || recent != "b" {
		t.Errorf("ReadRecentLog(1) = %q, %v", recent, err)
	}
}

func TestLoadWithUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	raw := "\ufeff[办公室]\nDHCP=1\n"
	if err := os.WriteFile(filepath.Join(dir, ProfileFile), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || len(got) != 1 || got[0].Name != "办公室" {
		t.Fatalf("BOM Load() = %+v, %v", got, err)
	}
}
