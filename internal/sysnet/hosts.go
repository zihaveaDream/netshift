package sysnet

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// DefaultHostsPath returns the Windows system hosts file path.
func DefaultHostsPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "drivers", "etc", "hosts")
}

// HostEntry is one parsed mapping row in a hosts file.
type HostEntry struct {
	Enabled bool
	IP      string
	Host    string
	Comment string
	lineIdx int // original index in HostFile.lines (-1 for newly added entries)
}

// HostFile keeps all original lines intact so toggling or adding mappings
// never destroys existing user comments or formatting.
type HostFile struct {
	Path    string
	lines   []string
	Entries []HostEntry
}

// LoadHosts reads and parses a hosts file.
func LoadHosts(path string) (*HostFile, error) {
	if path == "" {
		path = DefaultHostsPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 hosts 文件失败：%w", err)
	}
	return ParseHosts(path, string(raw)), nil
}

// ParseHosts parses hosts file content into a HostFile preserving raw lines.
func ParseHosts(path, content string) *HostFile {
	content = strings.TrimPrefix(content, "\ufeff")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	hf := &HostFile{Path: path, lines: lines}
	for i, rawLine := range lines {
		if entry, ok := parseHostLine(rawLine, i); ok {
			hf.Entries = append(hf.Entries, entry)
		}
	}
	return hf
}

// parseHostLine recognizes active ("127.0.0.1 app.local # note") and commented
// ("# 127.0.0.1 app.local # note") mapping lines while skipping pure prose comments.
func parseHostLine(line string, idx int) (HostEntry, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return HostEntry{}, false
	}
	enabled := true
	body := trimmed
	if strings.HasPrefix(body, "#") {
		enabled = false
		body = strings.TrimSpace(strings.TrimLeft(body, "#"))
	}
	if body == "" {
		return HostEntry{}, false
	}
	comment := ""
	if ci := strings.IndexByte(body, '#'); ci >= 0 {
		comment = strings.TrimSpace(body[ci+1:])
		body = strings.TrimSpace(body[:ci])
	}
	fields := strings.Fields(body)
	if len(fields) < 2 {
		return HostEntry{}, false
	}
	ip := net.ParseIP(fields[0])
	if ip == nil {
		return HostEntry{}, false
	}
	return HostEntry{
		Enabled: enabled,
		IP:      fields[0],
		Host:    strings.Join(fields[1:], " "),
		Comment: comment,
		lineIdx: idx,
	}, true
}

func formatHostEntry(e HostEntry) string {
	var b strings.Builder
	if !e.Enabled {
		b.WriteString("# ")
	}
	b.WriteString(strings.TrimSpace(e.IP))
	b.WriteString("\t")
	b.WriteString(strings.TrimSpace(e.Host))
	if c := strings.TrimSpace(e.Comment); c != "" {
		b.WriteString(" # ")
		b.WriteString(c)
	}
	return b.String()
}

// ValidateHostEntry checks IP and hostname fields before adding/saving.
func ValidateHostEntry(ip, host string) error {
	ip = strings.TrimSpace(ip)
	host = strings.TrimSpace(host)
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("无效的 IP 地址：%s", ip)
	}
	if host == "" {
		return fmt.Errorf("域名不能为空")
	}
	if strings.ContainsAny(host, "#\r\n") {
		return fmt.Errorf("域名不能包含 # 或换行")
	}
	return nil
}

// Add appends a new enabled mapping entry.
func (hf *HostFile) Add(ip, host, comment string) error {
	if err := ValidateHostEntry(ip, host); err != nil {
		return err
	}
	hf.Entries = append(hf.Entries, HostEntry{
		Enabled: true,
		IP:      strings.TrimSpace(ip),
		Host:    strings.TrimSpace(host),
		Comment: strings.TrimSpace(comment),
		lineIdx: -1,
	})
	return nil
}

// Delete removes the entry at index idx.
func (hf *HostFile) Delete(idx int) {
	if idx < 0 || idx >= len(hf.Entries) {
		return
	}
	e := hf.Entries[idx]
	if e.lineIdx >= 0 && e.lineIdx < len(hf.lines) {
		hf.lines[e.lineIdx] = "\x00DELETED"
	}
	hf.Entries = append(hf.Entries[:idx], hf.Entries[idx+1:]...)
}

// Render rebuilds the hosts file text, preserving non-mapping comments and blank lines.
func (hf *HostFile) Render() string {
	outLines := make([]string, len(hf.lines))
	copy(outLines, hf.lines)
	var extra []string
	for _, e := range hf.Entries {
		formatted := formatHostEntry(e)
		if e.lineIdx >= 0 && e.lineIdx < len(outLines) {
			outLines[e.lineIdx] = formatted
		} else {
			extra = append(extra, formatted)
		}
	}
	var final []string
	for _, l := range outLines {
		if l == "\x00DELETED" {
			continue
		}
		final = append(final, l)
	}
	final = append(final, extra...)
	return strings.Join(final, "\r\n")
}

// Save writes the updated hosts file back to disk, creating a one-time .bak
// backup if none exists yet.
func (hf *HostFile) Save() error {
	if hf.Path == "" {
		hf.Path = DefaultHostsPath()
	}
	bak := hf.Path + ".netshift.bak"
	if _, err := os.Stat(bak); os.IsNotExist(err) {
		if orig, err := os.ReadFile(hf.Path); err == nil {
			_ = os.WriteFile(bak, orig, 0o644)
		}
	}
	if err := os.WriteFile(hf.Path, []byte(hf.Render()), 0o644); err != nil {
		return fmt.Errorf("写入 hosts 文件失败（需要管理员权限）：%w", err)
	}
	return nil
}
