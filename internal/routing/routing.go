// Package routing builds the static routes of the split-routing screen: the
// default route goes away, one route per internal network is added, and a
// network equal to the adapter's own is reached directly.
package routing

import (
	"fmt"
	"net"
	"strings"

	"netshift/internal/model"
)

// Candidate is one internal network the screen offers.
type Candidate struct {
	Net, Mask string
	Label     string
}

// MaskOption is one netmask the screen offers.
type MaskOption struct {
	Mask  string
	Label string
}

// Masks are the masks the batch script offers, in its order.
func Masks() []MaskOption {
	pairs := []struct{ mask, label string }{
		{"255.255.255.0", "/24（约 254 台主机）"},
		{"255.255.0.0", "/16（约 6.5 万台）"},
		{"255.0.0.0", "/8（整个 A 类）"},
		{"255.255.255.128", "/25"},
		{"255.255.255.192", "/26"},
		{"255.255.255.224", "/27"},
		{"255.255.255.240", "/28"},
		{"255.255.254.0", "/23"},
		{"255.255.252.0", "/22"},
		{"255.255.240.0", "/20"},
		{"255.240.0.0", "/12"},
	}
	out := make([]MaskOption, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, MaskOption{Mask: p.mask, Label: p.label})
	}
	return out
}

// Candidates returns the networks the screen offers for ip: its own /16, /24
// and /8 first, then the private ranges the batch script lists. Duplicates
// keep their first position.
func Candidates(ip string) []Candidate {
	var out []Candidate
	seen := map[string]bool{}
	add := func(network, mask string) {
		key := network + "/" + mask
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Candidate{Net: network, Mask: mask, Label: fmt.Sprintf("%s（掩码 %s）", network, mask)})
	}
	if a := net.ParseIP(strings.TrimSpace(ip)).To4(); a != nil {
		add(fmt.Sprintf("%d.%d.0.0", a[0], a[1]), "255.255.0.0")
		add(fmt.Sprintf("%d.%d.%d.0", a[0], a[1], a[2]), "255.255.255.0")
		add(fmt.Sprintf("%d.0.0.0", a[0]), "255.0.0.0")
	}
	add("192.168.0.0", "255.255.0.0")
	add("172.16.0.0", "255.240.0.0")
	add("10.0.0.0", "255.0.0.0")
	return out
}

// Plan is one split route to apply.
type Plan struct {
	Adapter     string
	IfIndex     int
	Net         string
	Mask        string
	Gateway     string
	Direct      bool // the network is the adapter's own: reach it directly
	KeepDefault bool
	Persist     bool // keep the route after a restart (-p)
}

// Validate checks the plan before any command runs.
func (p Plan) Validate() error {
	netStr := strings.TrimSpace(p.Net)
	if netStr == "" {
		return fmt.Errorf("目标网段不能为空")
	}
	if err := model.ValidateIPv4(netStr); err != nil {
		return fmt.Errorf("目标网段格式应为 a.b.c.0：%s", p.Net)
	}
	if err := model.ValidateMask(p.Mask); err != nil {
		return err
	}
	gw := strings.TrimSpace(p.Gateway)
	if gw != "" {
		if err := model.ValidateIPv4(gw); err != nil {
			return fmt.Errorf("下一跳网关：%w", err)
		}
	}
	if !p.Direct && gw == "" {
		return fmt.Errorf("下一跳网关不能为空")
	}
	if strings.ContainsAny(p.Adapter, "\"&|<>^%\r\n") {
		return fmt.Errorf("网卡名含有不支持的字符：%s", p.Adapter)
	}
	return nil
}

// Commands returns the route commands of the plan, in the batch script's
// order: delete the default route through the gateway, then add the route
// unless the network is the adapter's own.
func (p Plan) Commands() ([]string, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	netStr := strings.TrimSpace(p.Net)
	maskStr := strings.TrimSpace(p.Mask)
	gwStr := strings.TrimSpace(p.Gateway)
	var out []string
	if gwStr != "" && !p.KeepDefault {
		out = append(out, fmt.Sprintf(`route delete 0.0.0.0 mask 0.0.0.0 %s`, gwStr))
	}
	if p.Direct {
		return out, nil
	}
	persist := ""
	if p.Persist {
		persist = " -p"
	}
	if p.IfIndex > 0 {
		out = append(out, fmt.Sprintf(`route add %s mask %s %s if %d%s`, netStr, maskStr, gwStr, p.IfIndex, persist))
	} else {
		out = append(out, fmt.Sprintf(`route add %s mask %s %s%s`, netStr, maskStr, gwStr, persist))
	}
	return out, nil
}

// IsDirect reports whether the route's network and mask are the adapter's own.
func IsDirect(adapterIP, adapterMask, network, mask string) bool {
	ip := net.ParseIP(strings.TrimSpace(adapterIP)).To4()
	if ip == nil {
		return false
	}
	netIP := net.ParseIP(strings.TrimSpace(network)).To4()
	if netIP == nil {
		return false
	}
	aMask := net.ParseIP(strings.TrimSpace(adapterMask)).To4()
	if aMask == nil {
		return false
	}
	ownNet := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		ownNet[i] = ip[i] & aMask[i]
	}
	return ownNet.String() == netIP.String() && strings.TrimSpace(adapterMask) == strings.TrimSpace(mask)
}

// RouteExists reports whether route print output already lists the route
// matching both destination network (column 0) and gateway (column 2).
func RouteExists(routePrint, network, gateway string) bool {
	wantNet := strings.TrimSpace(network)
	wantGW := strings.TrimSpace(gateway)
	for _, line := range strings.Split(routePrint, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		if f[0] == wantNet && f[2] == wantGW {
			return true
		}
	}
	return false
}

// RouteEntry is one parsed IPv4 route from `route print -4`.
type RouteEntry struct {
	Dest       string
	Mask       string
	Gateway    string
	Interface  string
	Metric     string
	Persistent bool
}

// ParseTable extracts active and persistent IPv4 routes from `route print -4`
// output (works on both Chinese and English Windows).
func ParseTable(routePrint string) []RouteEntry {
	var out []RouteEntry
	inPersistent := false
	for _, raw := range strings.Split(strings.ReplaceAll(routePrint, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.Contains(line, "永久路由") || strings.Contains(strings.ToLower(line), "persistent routes") {
			inPersistent = true
			continue
		}
		if strings.Contains(line, "活动路由") || strings.Contains(strings.ToLower(line), "active routes") {
			inPersistent = false
			continue
		}
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		if net.ParseIP(f[0]).To4() == nil || net.ParseIP(f[1]).To4() == nil {
			continue
		}
		if inPersistent {
			// Persistent route rows have 4 columns: Dest, Mask, Gateway, Metric.
			out = append(out, RouteEntry{
				Dest:       f[0],
				Mask:       f[1],
				Gateway:    f[2],
				Metric:     f[3],
				Persistent: true,
			})
		} else if len(f) >= 5 {
			// Active route rows have 5 columns: Dest, Mask, Gateway, Interface, Metric.
			// Note: localized "在链路上" or "On-link" may appear in column 2.
			gw := f[2]
			iface := f[3]
			metric := f[4]
			if len(f) > 5 {
				// Handle multi-word gateway like "On link" if ever present.
				metric = f[len(f)-1]
				iface = f[len(f)-2]
				gw = strings.Join(f[2:len(f)-2], " ")
			}
			out = append(out, RouteEntry{
				Dest:      f[0],
				Mask:      f[1],
				Gateway:   gw,
				Interface: iface,
				Metric:    metric,
			})
		}
	}
	return out
}

// DeleteCommand builds the Windows `route delete` command for e.
func DeleteCommand(e RouteEntry) (string, error) {
	if err := model.ValidateIPv4(e.Dest); err != nil {
		return "", fmt.Errorf("目标网段无效：%w", err)
	}
	if err := model.ValidateMask(e.Mask); err != nil {
		return "", fmt.Errorf("掩码无效：%w", err)
	}
	gw := strings.TrimSpace(e.Gateway)
	persist := ""
	if e.Persistent {
		persist = " -p"
	}
	if net.ParseIP(gw).To4() != nil {
		return fmt.Sprintf("route delete %s mask %s %s%s", strings.TrimSpace(e.Dest), strings.TrimSpace(e.Mask), gw, persist), nil
	}
	return fmt.Sprintf("route delete %s mask %s%s", strings.TrimSpace(e.Dest), strings.TrimSpace(e.Mask), persist), nil
}

// GatewayFor picks the gateway the screen offers by default: the adapter's,
// then the last one saved for the location, then the network's .1 address.
func GatewayFor(adapterGateways []string, savedForLocation string, network string) string {
	if len(adapterGateways) > 0 {
		return adapterGateways[0]
	}
	if savedForLocation != "" {
		return savedForLocation
	}
	if ip := net.ParseIP(strings.TrimSpace(network)).To4(); ip != nil {
		return fmt.Sprintf("%d.%d.%d.1", ip[0], ip[1], ip[2])
	}
	return ""
}
