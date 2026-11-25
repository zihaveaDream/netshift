// Package store reads and writes the app's files: ip.dat holds the saved
// profiles as INI sections, network_config.log the apply log, and
// ip_history.txt is the batch script's history file the app imports once.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"netshift/internal/model"
)

// File names, next to the executable as the original tool keeps them.
const (
	ProfileFile = "ip.dat"
	LogFile     = "network_config.log"
	LegacyFile  = "ip_history.txt"
)

// Store is the directory the app keeps its files in.
type Store struct {
	Dir string
}

// New returns a store for dir.
func New(dir string) *Store { return &Store{Dir: dir} }

func (s *Store) profilePath() string { return filepath.Join(s.Dir, ProfileFile) }
func (s *Store) legacyPath() string  { return filepath.Join(s.Dir, LegacyFile) }

// LogPath returns the full path of network_config.log.
func (s *Store) LogPath() string { return filepath.Join(s.Dir, LogFile) }

func (s *Store) backupPath(t time.Time) string {
	return filepath.Join(s.Dir, fmt.Sprintf("ip.dat.bak-%s", t.Format("20060102-150405")))
}

// Load reads the profiles from ip.dat. A missing file is not an error and
// yields no profiles. A file that does not parse is backed up and ignored, so
// a stale file never stops the app.
func (s *Store) Load() ([]model.Profile, error) {
	raw, err := os.ReadFile(s.profilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	profiles, perr := parseINI(string(raw))
	if perr != nil {
		_ = os.WriteFile(s.backupPath(time.Now()), raw, 0o644)
		return nil, fmt.Errorf("%s 无法解析（已备份）：%w", ProfileFile, perr)
	}
	model.SortProfiles(profiles)
	return profiles, nil
}

// Save writes the profiles to ip.dat. It writes a temporary file and renames
// it over the old one, so an interrupted save leaves the previous file whole.
func (s *Store) Save(profiles []model.Profile) error {
	var b strings.Builder
	b.WriteString("; NetShift 配置文件\n")
	b.WriteString("; 每个 [方案名] 是一份可应用的配置，日期格式为 RFC3339\n")
	for i, p := range profiles {
		p = p.Clone()
		if p.UpdatedAt.IsZero() {
			p.UpdatedAt = time.Now()
		}
		if i > 0 {
			b.WriteString("\n")
		}
		writeSection(&b, p)
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	tmp := s.profilePath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.profilePath()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func writeSection(b *strings.Builder, p model.Profile) {
	fmt.Fprintf(b, "[%s]\n", p.Name)
	kv := func(k, v string) {
		if v == "" {
			return
		}
		fmt.Fprintf(b, "%s=%s\n", k, v)
	}
	kv("Location", p.Location)
	kv("Adapter", p.Adapter)
	if p.DHCP {
		kv("DHCP", "1")
	} else {
		kv("DHCP", "0")
	}
	kv("IPv4", strings.Join(p.IPv4, "|"))
	kv("Masks", strings.Join(p.Masks, "|"))
	var gws []string
	for _, g := range p.Gateways {
		gws = append(gws, g.String())
	}
	kv("Gateways", strings.Join(gws, ";"))
	kv("DNS", strings.Join(p.DNS, ";"))
	kv("Computer", p.Computer)
	kv("Workgroup", p.Workgroup)
	kv("MAC", p.MAC)
	kv("Usage", fmt.Sprint(p.Usage))
	kv("UpdatedAt", p.UpdatedAt.Format(time.RFC3339))
}

// parseINI reads the section-per-profile INI the app writes. Unknown keys are
// ignored so a newer file still loads. Strips UTF-8 BOM if present.
func parseINI(raw string) ([]model.Profile, error) {
	raw = strings.TrimPrefix(raw, "\ufeff")
	var (
		out []model.Profile
		cur *model.Profile
	)
	lineNo := 0
	for _, line := range strings.Split(raw, "\n") {
		lineNo++
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, fmt.Errorf("第 %d 行缺少 ]", lineNo)
			}
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &model.Profile{Name: strings.TrimSpace(line[1 : len(line)-1])}
			continue
		}
		if cur == nil {
			return nil, fmt.Errorf("第 %d 行键值出现在任何 [方案] 之前", lineNo)
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			return nil, fmt.Errorf("第 %d 行缺少 =", lineNo)
		}
		key := strings.ToLower(strings.TrimSpace(line[:i]))
		val := strings.TrimSpace(line[i+1:])
		switch key {
		case "location":
			cur.Location = val
		case "adapter":
			cur.Adapter = val
		case "dhcp":
			cur.DHCP = val == "1" || strings.EqualFold(val, "true")
		case "ipv4":
			cur.IPv4 = splitList(val, "|")
		case "masks":
			cur.Masks = splitList(val, "|")
		case "gateways":
			for _, s := range splitList(val, ";") {
				if g, err := model.ParseGateway(s); err == nil {
					cur.Gateways = append(cur.Gateways, g)
				}
			}
		case "dns":
			cur.DNS = splitList(val, ";")
		case "computer":
			cur.Computer = val
		case "workgroup":
			cur.Workgroup = val
		case "mac":
			cur.MAC = val
		case "usage":
			fmt.Sscanf(val, "%d", &cur.Usage)
		case "updatedat":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				cur.UpdatedAt = t
			}
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out, nil
}

func splitList(s, sep string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// LegacyExists reports whether the batch script's history file is present.
func (s *Store) LegacyExists() bool {
	_, err := os.Stat(s.legacyPath())
	return err == nil
}

// ImportLegacy turns ip_history.txt into profiles. The file's lines hold
// 地点,系统名,网卡,IP,掩码,网关,首选DNS,备用DNS with 未填写 for empty values;
// older 7-field lines have no 系统名.
func (s *Store) ImportLegacy() ([]model.Profile, error) {
	raw, err := os.ReadFile(s.legacyPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	text := strings.TrimPrefix(string(raw), "\ufeff")
	var out []model.Profile
	used := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		f := strings.Split(line, ",")
		for i := range f {
			v := strings.TrimSpace(f[i])
			if v == "未填写" {
				v = ""
			}
			f[i] = v
		}
		var site, sys, adapter, ip, mask, gw, dns1, dns2 string
		switch len(f) {
		case 7: // 7-field: 地点,网卡,IP,掩码,网关,首选DNS,备用DNS
			site, adapter, ip, mask, gw, dns1, dns2 = f[0], f[1], f[2], f[3], f[4], f[5], f[6]
		default:
			if len(f) < 8 {
				continue
			}
			site, sys, adapter, ip, mask, gw, dns1, dns2 = f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7]
		}
		if ip == "" {
			continue
		}
		name := site
		if sys != "" {
			name = site + " " + sys
		}
		if name == "" {
			name = ip
		}
		base := name
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s (%d)", base, n)
		}
		used[name] = true
		p := model.Profile{
			Name:     name,
			Location: site,
			Adapter:  adapter,
			IPv4:     []string{ip},
			Masks:    []string{mask},
			DNS:      nil,
			Computer: sys,
			Usage:    1,
		}
		if gw != "" {
			p.Gateways = []model.Gateway{{Addr: gw}}
		}
		if dns1 != "" {
			p.DNS = append(p.DNS, dns1)
		}
		if dns2 != "" {
			p.DNS = append(p.DNS, dns2)
		}
		out = append(out, p)
	}
	return out, nil
}

// BackupLegacy renames ip_history.txt so the import happens only once.
func (s *Store) BackupLegacy(t time.Time) error {
	src := s.legacyPath()
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	dst := filepath.Join(s.Dir, fmt.Sprintf("ip_history.%s.txt", t.Format("20060102-150405")))
	return os.Rename(src, dst)
}

// LogFormat renders one applied configuration the way the batch script writes
// it, so existing logs stay consistent.
func LogFormat(now time.Time, site, sys, adapter string, p model.Profile) string {
	var b strings.Builder
	b.WriteString("===============================\n")
	fmt.Fprintf(&b, "时间：%s\n", now.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "项目地点：%s\n", site)
	fmt.Fprintf(&b, "系统名称：%s\n", sys)
	fmt.Fprintf(&b, "网卡：%s\n", adapter)
	if p.DHCP {
		b.WriteString("获取方式：自动获取 IP 和 DNS\n")
		fmt.Fprintf(&b, "IP地址：%s\n", "DHCP")
		fmt.Fprintf(&b, "子网掩码：%s\n", "DHCP")
		fmt.Fprintf(&b, "网关：%s\n", "DHCP")
		fmt.Fprintf(&b, "首选 DNS：%s\n", "DHCP")
		fmt.Fprintf(&b, "备用 DNS：%s\n", "DHCP")
	} else {
		fmt.Fprintf(&b, "IP地址：%s\n", strings.Join(p.IPv4, " "))
		fmt.Fprintf(&b, "子网掩码：%s\n", strings.Join(p.Masks, " "))
		fmt.Fprintf(&b, "网关：%s\n", orUnset(p.PrimaryGateway()))
		fmt.Fprintf(&b, "首选 DNS：%s\n", orUnset(p.PrimaryDNS()))
		fmt.Fprintf(&b, "备用 DNS：%s\n", orUnset(p.SecondaryDNS()))
	}
	b.WriteString("===============================\n")
	return b.String()
}

// LogRoute renders one applied static route the way the batch script does.
func LogRoute(now time.Time, site, sys, adapter, net, mask, gw, note string) string {
	var b strings.Builder
	b.WriteString("===============================\n")
	fmt.Fprintf(&b, "时间：%s\n", now.Format("2006-01-02 15:04:05"))
	b.WriteString("操作：内网分流路由\n")
	fmt.Fprintf(&b, "项目地点：%s\n", site)
	if sys != "" {
		fmt.Fprintf(&b, "系统名称：%s\n", sys)
	}
	fmt.Fprintf(&b, "网卡：%s\n", adapter)
	fmt.Fprintf(&b, "目标网段：%s\n", net)
	fmt.Fprintf(&b, "子网掩码：%s\n", mask)
	fmt.Fprintf(&b, "下一跳网关：%s\n", gw)
	if note != "" {
		fmt.Fprintf(&b, "备注：%s\n", note)
	}
	b.WriteString("===============================\n")
	return b.String()
}

func orUnset(s string) string {
	if s == "" {
		return "未填写"
	}
	return s
}

// AppendLog appends text to the log file.
func (s *Store) AppendLog(text string) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

// ReadRecentLog returns up to the last maxLines lines of network_config.log.
// Missing file returns "", nil.
func (s *Store) ReadRecentLog(maxLines int) (string, error) {
	raw, err := os.ReadFile(s.LogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	text := strings.TrimPrefix(string(raw), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n"), nil
}

// Names returns the profile names, sorted.
func Names(profiles []model.Profile) []string {
	out := make([]string, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}
