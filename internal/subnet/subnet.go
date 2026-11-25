// Package subnet computes what the subnet mask calculator shows: the network
// and broadcast addresses, the usable host range and its size. Pure math, no
// I/O.
package subnet

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"

	"netshift/internal/model"
)

// Info is the result of a calculation.
type Info struct {
	IP        string
	Prefix    int
	Mask      string
	Network   string
	Broadcast string
	First     string
	Last      string
	Hosts     uint64 // usable host addresses
	Total     uint64 // addresses in the subnet, network and broadcast included
}

// ParseCIDR splits "192.168.1.10/24" into ip and prefix; ok is false if s has
// no slash or invalid prefix.
func ParseCIDR(s string) (ip string, prefix int, ok bool) {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, '/')
	if i <= 0 {
		return "", 0, false
	}
	p, err := strconv.Atoi(strings.TrimSpace(s[i+1:]))
	if err != nil || p < 0 || p > 32 {
		return "", 0, false
	}
	return strings.TrimSpace(s[:i]), p, true
}

// Compute calculates the subnet of ip/mask. A /31 and /32 are reported the
// way Windows reports them: /31 has 2 hosts, /32 has 1 (the address itself).
func Compute(ip, mask string) (Info, error) {
	ip = strings.TrimSpace(ip)
	addr := net.ParseIP(ip)
	if addr == nil || addr.To4() == nil {
		return Info{}, fmt.Errorf("IP 地址无效：%s", ip)
	}
	if err := model.ValidateMask(mask); err != nil {
		return Info{}, err
	}
	prefix := model.MaskPrefix(mask)
	if prefix < 0 {
		return Info{}, fmt.Errorf("子网掩码无效：%s", mask)
	}
	return ComputePrefix(ip, prefix)
}

// ComputePrefix calculates the subnet of ip/prefix.
func ComputePrefix(ip string, prefix int) (Info, error) {
	ip = strings.TrimSpace(ip)
	addr := net.ParseIP(ip)
	if addr == nil || addr.To4() == nil {
		return Info{}, fmt.Errorf("IP 地址无效：%s", ip)
	}
	if prefix < 0 || prefix > 32 {
		return Info{}, fmt.Errorf("前缀长度无效：%d", prefix)
	}
	mask := model.PrefixMask(prefix)
	maskIP := net.ParseIP(mask).To4()
	a := addr.To4()

	network := make(net.IP, 4)
	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		network[i] = a[i] & maskIP[i]
		broadcast[i] = network[i] | ^maskIP[i]
	}
	total := uint64(1) << uint(32-prefix)
	info := Info{
		IP:        a.String(),
		Prefix:    prefix,
		Mask:      mask,
		Network:   network.String(),
		Broadcast: broadcast.String(),
		Total:     total,
	}
	switch prefix {
	case 32:
		info.First = a.String()
		info.Last = a.String()
		info.Hosts = 1
	case 31:
		info.First = network.String()
		info.Last = broadcast.String()
		info.Hosts = 2
	default:
		first := make(net.IP, 4)
		last := make(net.IP, 4)
		copy(first, network)
		copy(last, broadcast)
		first[3]++
		last[3]--
		info.First = first.String()
		info.Last = last.String()
		info.Hosts = total - 2
	}
	return info, nil
}

// Range renders the usable host range as "first - last".
func (i Info) Range() string {
	if i.First == "" || i.Last == "" {
		return ""
	}
	return i.First + " - " + i.Last
}

// Covers reports whether the subnet contains addr.
func (i Info) Covers(addr string) bool {
	a := net.ParseIP(strings.TrimSpace(addr)).To4()
	n := net.ParseIP(i.Network).To4()
	b := net.ParseIP(i.Broadcast).To4()
	if a == nil || n == nil || b == nil {
		return false
	}
	v := binary.BigEndian.Uint32(a)
	lo := binary.BigEndian.Uint32(n)
	hi := binary.BigEndian.Uint32(b)
	return v >= lo && v <= hi
}
