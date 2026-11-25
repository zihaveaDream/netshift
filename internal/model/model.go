// Package model holds the data the app shows and stores: a profile (one
// saved configuration), its gateways, and the app-wide settings. It has no
// I/O and no Windows API, so it is testable anywhere.
package model

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Gateway is one default gateway with its interface metric.
type Gateway struct {
	Addr   string
	Metric int
}

// String renders "192.168.1.1|1".
func (g Gateway) String() string {
	if g.Metric <= 0 {
		return g.Addr
	}
	return fmt.Sprintf("%s|%d", g.Addr, g.Metric)
}

// ParseGateway reads "192.168.1.1" or "192.168.1.1|1".
func ParseGateway(s string) (Gateway, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Gateway{}, fmt.Errorf("网关为空")
	}
	addr, metric := s, 0
	if i := strings.LastIndexByte(s, '|'); i >= 0 {
		addr = strings.TrimSpace(s[:i])
		m := strings.TrimSpace(s[i+1:])
		if m != "" {
			v, err := strconv.Atoi(m)
			if err != nil {
				return Gateway{}, fmt.Errorf("跃点数不是数字：%s", m)
			}
			if v < 0 || v > 9999 {
				return Gateway{}, fmt.Errorf("跃点数应在 0..9999 之间：%d", v)
			}
			metric = v
		}
	}
	if err := ValidateIPv4(addr); err != nil {
		return Gateway{}, err
	}
	return Gateway{Addr: addr, Metric: metric}, nil
}

// Profile is one saved configuration: an INI section of ip.dat.
type Profile struct {
	Name      string    // section name, shown in the preset list
	Location  string    // project site, from the batch script's vocabulary
	Adapter   string    // connection name, e.g. 以太网
	DHCP      bool      // true: obtain an address automatically
	IPv4      []string  // static addresses, in order
	Masks     []string  // one per address
	Gateways  []Gateway // gateways with metrics, in order
	DNS       []string  // DNS servers, in order
	Computer  string    // computer name to apply, "" for none
	Workgroup string    // workgroup to apply, "" for none
	MAC       string    // MAC to apply, "" for none
	Usage     int       // how often it was applied, for recent-first order
	UpdatedAt time.Time // last save
}

// Clone returns a deep copy.
func (p Profile) Clone() Profile {
	c := p
	c.IPv4 = append([]string(nil), p.IPv4...)
	c.Masks = append([]string(nil), p.Masks...)
	c.Gateways = append([]Gateway(nil), p.Gateways...)
	c.DNS = append([]string(nil), p.DNS...)
	return c
}

// PrimaryIP returns the first address, or "".
func (p Profile) PrimaryIP() string {
	if len(p.IPv4) == 0 {
		return ""
	}
	return p.IPv4[0]
}

// PrimaryMask returns the mask of the first address, or "".
func (p Profile) PrimaryMask() string {
	if len(p.Masks) == 0 {
		return ""
	}
	return p.Masks[0]
}

// PrimaryGateway returns the first gateway address, or "".
func (p Profile) PrimaryGateway() string {
	if len(p.Gateways) == 0 {
		return ""
	}
	return p.Gateways[0].Addr
}

// PrimaryDNS returns the first DNS server, or "".
func (p Profile) PrimaryDNS() string {
	if len(p.DNS) == 0 {
		return ""
	}
	return p.DNS[0]
}

// SecondaryDNS returns the second DNS server, or "".
func (p Profile) SecondaryDNS() string {
	if len(p.DNS) < 2 {
		return ""
	}
	return p.DNS[1]
}

// Validate reports the first problem with the profile, in the words the UI
// shows. Details that a disabled branch needs are not checked.
func (p Profile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("方案名不能为空")
	}
	if strings.ContainsAny(p.Name, "[]=\r\n") {
		return fmt.Errorf("方案名不能包含 [ ] = 或换行")
	}
	if !p.DHCP {
		if len(p.IPv4) == 0 {
			return fmt.Errorf("请填写 IP 地址")
		}
		if len(p.Masks) < len(p.IPv4) {
			return fmt.Errorf("每个 IP 地址都要有子网掩码")
		}
		for i, ip := range p.IPv4 {
			if err := ValidateIPv4(ip); err != nil {
				return fmt.Errorf("第 %d 个 IP 地址：%w", i+1, err)
			}
		}
		for i, m := range p.Masks {
			if err := ValidateMask(m); err != nil {
				return fmt.Errorf("第 %d 个子网掩码：%w", i+1, err)
			}
		}
		for i, g := range p.Gateways {
			if err := ValidateIPv4(g.Addr); err != nil {
				return fmt.Errorf("第 %d 个网关：%w", i+1, err)
			}
			if g.Metric < 0 || g.Metric > 9999 {
				return fmt.Errorf("第 %d 个网关跃点数应在 0..9999 之间", i+1)
			}
		}
	}
	for i, d := range p.DNS {
		if err := ValidateIPv4(d); err != nil {
			return fmt.Errorf("第 %d 个 DNS：%w", i+1, err)
		}
	}
	return nil
}

// ValidateIPv4 reports whether s is an IPv4 address in dotted decimal form.
func ValidateIPv4(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("地址为空")
	}
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("不是合法的 IPv4 地址：%s", s)
	}
	if strings.Count(s, ".") != 3 {
		return fmt.Errorf("不是点分十进制：%s", s)
	}
	return nil
}

// ValidateMask reports whether s is a contiguous IPv4 netmask.
func ValidateMask(s string) error {
	s = strings.TrimSpace(s)
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil || strings.Count(s, ".") != 3 {
		return fmt.Errorf("不是合法的子网掩码：%s", s)
	}
	b := ip.To4()
	seenZero := false
	for _, v := range b {
		for bit := 7; bit >= 0; bit-- {
			one := v&(1<<uint(bit)) != 0
			if one && seenZero {
				return fmt.Errorf("子网掩码不连续：%s", s)
			}
			if !one {
				seenZero = true
			}
		}
	}
	return nil
}

// MaskPrefix returns the prefix length of a contiguous netmask, or -1.
func MaskPrefix(mask string) int {
	if err := ValidateMask(mask); err != nil {
		return -1
	}
	ip := net.ParseIP(strings.TrimSpace(mask)).To4()
	n := 0
	for _, v := range ip {
		for bit := 7; bit >= 0; bit-- {
			if v&(1<<uint(bit)) == 0 {
				return n
			}
			n++
		}
	}
	return n
}

// PrefixMask returns the dotted netmask of a prefix length.
func PrefixMask(prefix int) string {
	if prefix < 0 || prefix > 32 {
		return ""
	}
	var b [4]byte
	for i := 0; i < prefix; i++ {
		b[i/8] |= 1 << uint(7-i%8)
	}
	return net.IP(b[:]).String()
}

// SortProfiles orders profiles recent-first, then by name.
func SortProfiles(ps []Profile) {
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].Usage != ps[j].Usage {
			return ps[i].Usage > ps[j].Usage
		}
		return ps[i].Name < ps[j].Name
	})
}

// SameSite returns the profiles of a location.
func SameSite(ps []Profile, location string) []Profile {
	var out []Profile
	for _, p := range ps {
		if p.Location == location {
			out = append(out, p)
		}
	}
	return out
}

// Locations returns the distinct non-empty locations in order of appearance.
func Locations(ps []Profile) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range ps {
		if p.Location == "" || seen[p.Location] {
			continue
		}
		seen[p.Location] = true
		out = append(out, p.Location)
	}
	return out
}
