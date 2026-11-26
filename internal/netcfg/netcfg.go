// Package netcfg lists the machine's adapters and applies addresses to them,
// through the same netsh commands the original tool falls back to.
package netcfg

import (
	"fmt"
	"net"
	"strings"

	"netshift/internal/model"
)

// Adapter is one network connection.
type Adapter struct {
	GUID          string // NetCfgInstanceId, the registry key name
	Name          string // connection name, e.g. 以太网
	Description   string // driver description
	Index         int    // IPv4 interface index
	MAC           string // 12 hex digits, upper case, no separators
	IPv4          []string
	Masks         []string
	Gateways      []string
	DNS           []string
	DHCP          bool
	Up            bool
	IfType        uint32 // MIB_IF_TYPE_ETHERNET(6), IF_TYPE_IEEE80211(71), etc.
	LinkSpeedMbps uint64 // transmit link speed in Mbps (0 when unknown)
}

// TypeLabel returns a short human-readable adapter type.
func (a Adapter) TypeLabel() string {
	switch a.IfType {
	case 6:
		return "以太网"
	case 71:
		return "Wi-Fi"
	case 53:
		return "虚拟网卡"
	case 131:
		return "隧道"
	default:
		return "网络适配器"
	}
}

// SpeedLabel formats the link speed for display, or "" if unavailable.
func (a Adapter) SpeedLabel() string {
	if a.LinkSpeedMbps == 0 {
		return ""
	}
	if a.LinkSpeedMbps >= 1000 && a.LinkSpeedMbps%1000 == 0 {
		return fmt.Sprintf("%d Gbps", a.LinkSpeedMbps/1000)
	}
	return fmt.Sprintf("%d Mbps", a.LinkSpeedMbps)
}

// Status renders the connection state the way the main window shows it.
func (a Adapter) Status() string {
	switch {
	case !a.Up:
		return "已断开"
	case len(a.IPv4) == 0:
		return "无 IP"
	default:
		return "已连接"
	}
}

// CurrentIP returns the first address, or "".
func (a Adapter) CurrentIP() string {
	if len(a.IPv4) == 0 {
		return ""
	}
	return a.IPv4[0]
}

// UsableIPv4 returns the first address that can actually be used, skipping the
// 169.254.x.x address Windows assigns itself when DHCP hands out nothing.
func (a Adapter) UsableIPv4() string {
	for _, ip := range a.IPv4 {
		if net.ParseIP(strings.TrimSpace(ip)).To4() == nil || IsAPIPA(ip) {
			continue
		}
		return ip
	}
	return ""
}

// HasUsableIPv4 reports whether the adapter holds an address that is not an
// automatic private one.
func (a Adapter) HasUsableIPv4() bool { return a.UsableIPv4() != "" }

// IsAPIPA reports whether ip is an IPv4 link-local address (169.254.0.0/16), the
// one Windows picks when no DHCP server answered.
func IsAPIPA(ip string) bool {
	ip4 := net.ParseIP(strings.TrimSpace(ip)).To4()
	return ip4 != nil && ip4[0] == 169 && ip4[1] == 254
}

// Label is the line shown in the adapter list.
func (a Adapter) Label() string {
	ip := a.CurrentIP()
	if ip == "" {
		ip = "无 IP"
	}
	return fmt.Sprintf("%s (%s)", a.Name, ip)
}

// Request is one configuration to apply to an adapter.
type Request struct {
	Adapter  string
	DHCP     bool
	IPv4     []string
	Masks    []string
	Gateways []model.Gateway
	DNS      []string
}

// QuoteName quotes an adapter name for a netsh command line and rejects the
// characters that would let a name change the command.
func QuoteName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("网卡名不能为空")
	}
	if strings.ContainsAny(name, "\"&|<>^%\r\n") {
		return "", fmt.Errorf("网卡名含有不支持的字符：%s", name)
	}
	return `"` + name + `"`, nil
}

// AddressCommands returns the netsh lines that set r's addresses and
// gateways, without touching DNS. All gateways (including extra gateways beyond
// len(IPv4) or gateways without an explicit metric) are preserved.
func AddressCommands(r Request) ([]string, error) {
	name, err := QuoteName(r.Adapter)
	if err != nil {
		return nil, err
	}
	var out []string
	if r.DHCP {
		return []string{fmt.Sprintf(`netsh interface ip set address name=%s source=dhcp`, name)}, nil
	}
	if len(r.IPv4) == 0 {
		return nil, fmt.Errorf("没有 IP 地址")
	}
	for i, g := range r.Gateways {
		if strings.TrimSpace(g.Addr) == "" {
			continue
		}
		if err := model.ValidateIPv4(g.Addr); err != nil {
			return nil, fmt.Errorf("第 %d 个网关：%w", i+1, err)
		}
		if g.Metric < 0 || g.Metric > 9999 {
			return nil, fmt.Errorf("第 %d 个网关跃点数应在 0..9999 之间", i+1)
		}
	}
	for i, ip := range r.IPv4 {
		ip = strings.TrimSpace(ip)
		if err := model.ValidateIPv4(ip); err != nil {
			return nil, err
		}
		if i >= len(r.Masks) {
			return nil, fmt.Errorf("第 %d 个 IP 地址没有子网掩码", i+1)
		}
		mask := strings.TrimSpace(r.Masks[i])
		if err := model.ValidateMask(mask); err != nil {
			return nil, err
		}
		gw, metric := "", 0
		if i < len(r.Gateways) {
			gw = strings.TrimSpace(r.Gateways[i].Addr)
			metric = r.Gateways[i].Metric
		}
		switch {
		case i == 0 && gw != "" && metric > 0:
			out = append(out, fmt.Sprintf(`netsh interface ip set address name=%s static %s %s %s gwmetric=%d`, name, ip, mask, gw, metric))
		case i == 0 && gw != "":
			out = append(out, fmt.Sprintf(`netsh interface ip set address name=%s static %s %s %s`, name, ip, mask, gw))
		case i == 0:
			out = append(out, fmt.Sprintf(`netsh interface ip set address name=%s static %s %s`, name, ip, mask))
		case gw != "" && metric > 0:
			out = append(out, fmt.Sprintf(`netsh interface ip add address name=%s %s %s %s gwmetric=%d`, name, ip, mask, gw, metric))
		case gw != "":
			out = append(out, fmt.Sprintf(`netsh interface ip add address name=%s %s %s %s`, name, ip, mask, gw))
		default:
			out = append(out, fmt.Sprintf(`netsh interface ip add address name=%s %s %s`, name, ip, mask))
		}
	}
	// Additional gateways beyond the number of static IPv4 addresses.
	for i := len(r.IPv4); i < len(r.Gateways); i++ {
		gw := strings.TrimSpace(r.Gateways[i].Addr)
		if gw == "" {
			continue
		}
		if r.Gateways[i].Metric > 0 {
			out = append(out, fmt.Sprintf(`netsh interface ip add address name=%s gateway=%s gwmetric=%d`, name, gw, r.Gateways[i].Metric))
		} else {
			out = append(out, fmt.Sprintf(`netsh interface ip add address name=%s gateway=%s gwmetric=0`, name, gw))
		}
	}
	return out, nil
}

// Commands returns the netsh command lines that apply r, in order.
func Commands(r Request) ([]string, error) {
	name, err := QuoteName(r.Adapter)
	if err != nil {
		return nil, err
	}
	out, err := AddressCommands(r)
	if err != nil {
		return nil, err
	}
	if r.DHCP {
		out = append(out, fmt.Sprintf(`netsh interface ip set dns name=%s source=dhcp`, name))
		return out, nil
	}
	dnsCmds, err := SetDNSCommands(r.Adapter, r.DNS)
	if err != nil {
		return nil, err
	}
	out = append(out, dnsCmds...)
	return out, nil
}

// SetEnabled returns the command that enables or disables an adapter.
func SetEnabled(adapter string, on bool) (string, error) {
	name, err := QuoteName(adapter)
	if err != nil {
		return "", err
	}
	admin := "disabled"
	if on {
		admin = "enabled"
	}
	return fmt.Sprintf(`netsh interface set interface name=%s admin=%s`, name, admin), nil
}

// SetMetric returns the command that sets an adapter's interface metric.
func SetMetric(adapter string, metric int) (string, error) {
	name, err := QuoteName(adapter)
	if err != nil {
		return "", err
	}
	if metric < 1 || metric > 9999 {
		return "", fmt.Errorf("跃点数应在 1..9999 之间")
	}
	return fmt.Sprintf(`netsh interface ip set interface interface=%s metric=%d`, name, metric), nil
}

// SetGatewayCommand changes only the default gateway of an adapter, keeping
// its address: the form netsh accepts for that is address ... gateway=.
func SetGatewayCommand(adapter, gateway string, metric int) (string, error) {
	name, err := QuoteName(adapter)
	if err != nil {
		return "", err
	}
	gateway = strings.TrimSpace(gateway)
	if err := model.ValidateIPv4(gateway); err != nil {
		return "", err
	}
	if metric > 0 {
		return fmt.Sprintf(`netsh interface ip set address name=%s gateway=%s gwmetric=%d`, name, gateway, metric), nil
	}
	return fmt.Sprintf(`netsh interface ip set address name=%s gateway=%s`, name, gateway), nil
}

// SetGatewaysCommands builds the commands to replace an adapter's gateways with
// one or more entries, keeping its IPv4 addresses untouched.
func SetGatewaysCommands(adapter string, gateways []model.Gateway) ([]string, error) {
	name, err := QuoteName(adapter)
	if err != nil {
		return nil, err
	}
	if len(gateways) == 0 {
		return nil, fmt.Errorf("网关列表为空")
	}
	var out []string
	for i, g := range gateways {
		gw := strings.TrimSpace(g.Addr)
		if err := model.ValidateIPv4(gw); err != nil {
			return nil, fmt.Errorf("第 %d 个网关：%w", i+1, err)
		}
		if g.Metric < 0 || g.Metric > 9999 {
			return nil, fmt.Errorf("第 %d 个网关跃点数应在 0..9999 之间", i+1)
		}
		if i == 0 {
			cmd, err := SetGatewayCommand(adapter, gw, g.Metric)
			if err != nil {
				return nil, err
			}
			out = append(out, cmd)
		} else if g.Metric > 0 {
			out = append(out, fmt.Sprintf(`netsh interface ip add address name=%s gateway=%s gwmetric=%d`, name, gw, g.Metric))
		} else {
			out = append(out, fmt.Sprintf(`netsh interface ip add address name=%s gateway=%s gwmetric=0`, name, gw))
		}
	}
	return out, nil
}

// SetDNSCommands returns the commands that set an adapter's DNS servers and
// remove the ones it had.
func SetDNSCommands(adapter string, servers []string) ([]string, error) {
	name, err := QuoteName(adapter)
	if err != nil {
		return nil, err
	}
	var clean []string
	for _, s := range servers {
		if s = strings.TrimSpace(s); s != "" {
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 {
		return []string{fmt.Sprintf(`netsh interface ip set dns name=%s source=dhcp`, name)}, nil
	}
	var out []string
	for i, d := range clean {
		if err := model.ValidateIPv4(d); err != nil {
			return nil, err
		}
		if i == 0 {
			out = append(out, fmt.Sprintf(`netsh interface ip set dns name=%s static %s validate=no primary`, name, d))
		} else {
			out = append(out, fmt.Sprintf(`netsh interface ip add dns name=%s %s index=%d validate=no`, name, d, i+1))
		}
	}
	return out, nil
}

// ReleaseCommand, RenewCommand and FlushDNSCommand are the ipconfig calls the
// original tool runs from its advanced options page.
func ReleaseCommand(adapter string) (string, error) {
	name, err := QuoteName(adapter)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`ipconfig /release %s`, name), nil
}

func RenewCommand(adapter string) (string, error) {
	name, err := QuoteName(adapter)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`ipconfig /renew %s`, name), nil
}

func FlushDNSCommand() string { return `ipconfig /flushdns` }

// checkNetshResult reconciles two Windows netsh quirks:
//  1. Invalid arguments or syntax errors print an error/Usage block yet still
//     exit with code 0, which would otherwise look like a silent success.
//  2. Running `source=dhcp` on an interface/DNS that is already set to DHCP
//     prints "DHCP is already enabled..." (or its localized equivalent) and
//     exits with code 1, which is idempotently already in the desired state.
func checkNetshResult(line, output string, runErr error) error {
	trimmedLine := strings.TrimSpace(line)
	if !strings.HasPrefix(strings.ToLower(trimmedLine), "netsh ") {
		return runErr
	}
	if runErr != nil {
		if isAlreadyDHCP(trimmedLine, output) {
			return nil
		}
		return runErr
	}
	if isNetshSyntaxError(output) {
		return fmt.Errorf("netsh 命令参数或语法错误：%s", firstNonEmptyLine(output))
	}
	return nil
}

func isAlreadyDHCP(line, output string) bool {
	if !strings.Contains(strings.ToLower(line), "source=dhcp") {
		return false
	}
	lower := strings.ToLower(output)
	return strings.Contains(lower, "dhcp is already enabled") ||
		strings.Contains(lower, "已启用 dhcp") ||
		strings.Contains(lower, "已启用dhcp")
}

func isNetshSyntaxError(output string) bool {
	if strings.TrimSpace(output) == "" {
		return false
	}
	lower := strings.ToLower(output)
	for _, marker := range []string{
		"is not a valid argument for this command",
		"the syntax supplied for this command is not valid",
		"the following command was not found",
		"不是此命令的有效参数",
		"为此命令提供的语法不正确",
		"找不到以下命令",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, raw := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(strings.ToLower(line), "usage:") ||
			strings.HasPrefix(line, "用法:") ||
			strings.HasPrefix(line, "用法：") {
			return true
		}
	}
	return false
}

func firstNonEmptyLine(s string) string {
	for _, raw := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if line := strings.TrimSpace(raw); line != "" {
			return line
		}
	}
	return strings.TrimSpace(s)
}
