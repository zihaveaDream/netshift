package model

import (
	"strings"
	"testing"
)

func TestValidateIPv4(t *testing.T) {
	good := []string{"192.168.1.1", "0.0.0.0", "255.255.255.255", "10.0.0.5"}
	for _, s := range good {
		if err := ValidateIPv4(s); err != nil {
			t.Errorf("%s rejected: %v", s, err)
		}
	}
	bad := []string{"", "192.168.1", "192.168.1.256", "abc", "192.168.1.1.1", "::1"}
	for _, s := range bad {
		if err := ValidateIPv4(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestValidateMask(t *testing.T) {
	good := map[string]int{
		"255.255.255.0": 24, "255.255.0.0": 16, "255.0.0.0": 8,
		"255.255.255.252": 30, "255.255.255.254": 31, "255.255.255.255": 32,
		"0.0.0.0": 0,
	}
	for mask, prefix := range good {
		if err := ValidateMask(mask); err != nil {
			t.Errorf("%s rejected: %v", mask, err)
			continue
		}
		if got := MaskPrefix(mask); got != prefix {
			t.Errorf("MaskPrefix(%s) = %d, want %d", mask, got, prefix)
		}
		if got := PrefixMask(prefix); got != mask && prefix != 0 {
			t.Errorf("PrefixMask(%d) = %s, want %s", prefix, got, mask)
		}
	}
	if err := ValidateMask("255.255.255.255"); err != nil {
		t.Error("a /32 mask should be valid")
	}
	for _, mask := range []string{"255.0.255.0", "255.255.255.1", "abc"} {
		if err := ValidateMask(mask); err == nil {
			t.Errorf("%s accepted", mask)
		}
	}
}

func TestProfileValidate(t *testing.T) {
	p := Profile{Name: "办公室", IPv4: []string{"192.168.1.10"}, Masks: []string{"255.255.255.0"}}
	if err := p.Validate(); err != nil {
		t.Errorf("valid profile rejected: %v", err)
	}
	p.Name = ""
	if err := p.Validate(); err == nil {
		t.Error("empty name accepted")
	}
	p.Name = "a[b"
	if err := p.Validate(); err == nil {
		t.Error("bracket in name accepted")
	}
	p = Profile{Name: "x", IPv4: []string{"192.168.1.10"}}
	if err := p.Validate(); err == nil {
		t.Error("missing mask accepted")
	}
	p = Profile{Name: "x", DHCP: true}
	if err := p.Validate(); err != nil {
		t.Errorf("a DHCP profile needs no addresses: %v", err)
	}
	p = Profile{Name: "x", IPv4: []string{"1.2.3.4"}, Masks: []string{"255.0.0.0"}, DNS: []string{"nope"}}
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "DNS") {
		t.Errorf("bad DNS accepted: %v", err)
	}
}

func TestParseGateway(t *testing.T) {
	g, err := ParseGateway("192.168.1.1")
	if err != nil || g.Addr != "192.168.1.1" || g.Metric != 0 {
		t.Errorf("plain gateway = %+v, %v", g, err)
	}
	g, err = ParseGateway("192.168.1.1|7")
	if err != nil || g.Metric != 7 {
		t.Errorf("metric gateway = %+v, %v", g, err)
	}
	if got := g.String(); got != "192.168.1.1|7" {
		t.Errorf("String() = %s", got)
	}
	if _, err := ParseGateway("192.168.1.1|x"); err == nil {
		t.Error("bad metric accepted")
	}
	if _, err := ParseGateway(""); err == nil {
		t.Error("empty gateway accepted")
	}
}

func TestSortProfilesAndSites(t *testing.T) {
	ps := []Profile{
		{Name: "b", Usage: 1, Location: "北京"},
		{Name: "a", Usage: 5, Location: "北京"},
		{Name: "c", Usage: 1, Location: "上海"},
	}
	SortProfiles(ps)
	if ps[0].Name != "a" || ps[1].Name != "b" || ps[2].Name != "c" {
		t.Errorf("order = %v", []string{ps[0].Name, ps[1].Name, ps[2].Name})
	}
	if got := Locations(ps); len(got) != 2 || got[0] != "北京" || got[1] != "上海" {
		t.Errorf("locations = %v", got)
	}
	if got := SameSite(ps, "北京"); len(got) != 2 {
		t.Errorf("same site = %d", len(got))
	}
}

func TestProfileAccessors(t *testing.T) {
	p := Profile{
		IPv4:     []string{"192.168.1.10", "10.0.0.5"},
		Masks:    []string{"255.255.255.0", "255.0.0.0"},
		Gateways: []Gateway{{Addr: "192.168.1.1"}},
		DNS:      []string{"192.168.1.1", "8.8.8.8"},
	}
	if p.PrimaryIP() != "192.168.1.10" || p.PrimaryMask() != "255.255.255.0" {
		t.Errorf("primary address = %s/%s", p.PrimaryIP(), p.PrimaryMask())
	}
	if p.PrimaryGateway() != "192.168.1.1" || p.PrimaryDNS() != "192.168.1.1" || p.SecondaryDNS() != "8.8.8.8" {
		t.Errorf("gateway/DNS accessors wrong")
	}
	c := p.Clone()
	c.IPv4[0] = "changed"
	if p.IPv4[0] != "192.168.1.10" {
		t.Error("Clone shares the IPv4 slice")
	}
}
