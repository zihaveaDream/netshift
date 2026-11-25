//go:build windows

// Package macaddr reads, sets and restores the MAC address of an adapter the
// way Windows supports it: the NetworkAddress parameter of the adapter's class
// key, followed by a restart of the adapter.
package macaddr

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	"netshift/internal/netcfg"
)

// classKey is where Windows keeps the per-adapter driver parameters.
const classKey = `SYSTEM\CurrentControlSet\Control\Class\{4d36e972-e325-11ce-bfc1-08002be10318}`

// Normalize validates a unicast MAC address and returns it as 12 upper-case hex
// digits without separators. Forms accepted: AA-BB-CC-DD-EE-FF, AA:BB:..., AABBCCDDEEFF.
func Normalize(s string) (string, error) {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case '-', ':', ' ', '.':
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len(clean) != 12 {
		return "", fmt.Errorf("MAC 地址应为 12 位十六进制：%s", s)
	}
	var out strings.Builder
	out.Grow(12)
	for _, r := range strings.ToUpper(clean) {
		if (r < '0' || r > '9') && (r < 'A' || r > 'F') {
			return "", fmt.Errorf("MAC 地址含有非十六进制字符：%s", s)
		}
		out.WriteRune(r)
	}
	v := out.String()
	if v == "000000000000" {
		return "", fmt.Errorf("MAC 地址不能全为 0")
	}
	if v == "FFFFFFFFFFFF" {
		return "", fmt.Errorf("MAC 地址不能为广播地址 FF-FF-FF-FF-FF-FF")
	}
	firstByte, err := strconv.ParseUint(v[:2], 16, 8)
	if err != nil || firstByte&0x01 != 0 {
		return "", fmt.Errorf("MAC 地址首字节最低位必须为 0（单播地址）：%s", s)
	}
	return v, nil
}

// Format renders a 12-hex MAC (or separated MAC) as AA-BB-CC-DD-EE-FF.
func Format(mac string) string {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case '-', ':', ' ', '.':
			return -1
		}
		return r
	}, strings.TrimSpace(mac))
	if len(clean) != 12 {
		return mac
	}
	upper := strings.ToUpper(clean)
	return upper[0:2] + "-" + upper[2:4] + "-" + upper[4:6] + "-" + upper[6:8] + "-" + upper[8:10] + "-" + upper[10:12]
}

// Random returns a random unicast, locally administered MAC address.
func Random() string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	b[0] = (b[0] | 0x02) &^ 0x01 // locally administered, unicast
	var sb strings.Builder
	sb.Grow(12)
	for _, v := range b {
		fmt.Fprintf(&sb, "%02X", v)
	}
	return sb.String()
}

// Set writes mac as the adapter's NetworkAddress. The adapter is restarted for
// Windows to apply it.
func Set(adapter netcfg.Adapter, mac string) error {
	normalized, err := Normalize(mac)
	if err != nil {
		return err
	}
	sub, err := findClassSubkey(adapter.GUID)
	if err != nil {
		return err
	}
	base := classKey + `\` + sub

	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, base, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表项失败：%w", err)
	}
	err = key.SetStringValue("NetworkAddress", normalized)
	key.Close()
	if err != nil {
		return fmt.Errorf("写入 MAC 失败：%w", err)
	}
	return restartAdapter(adapter.Name)
}

// Restore removes a MAC address set earlier, so the adapter uses its
// permanent burned-in address again.
func Restore(adapter netcfg.Adapter) error {
	sub, err := findClassSubkey(adapter.GUID)
	if err != nil {
		return err
	}
	base := classKey + `\` + sub
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, base, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表项失败：%w", err)
	}
	_ = key.DeleteValue("NetworkAddress")
	key.Close()
	return restartAdapter(adapter.Name)
}

// Configured returns the NetworkAddress set for the adapter, or "".
func Configured(adapter netcfg.Adapter) string {
	sub, err := findClassSubkey(adapter.GUID)
	if err != nil {
		return ""
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, classKey+`\`+sub, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	if v, _, err := key.GetStringValue("NetworkAddress"); err == nil && v != "" {
		return strings.ToUpper(v)
	}
	return ""
}

// findClassSubkey finds the 0000..NNNN subkey whose NetCfgInstanceId is the
// adapter's GUID.
func findClassSubkey(guid string) (string, error) {
	if guid == "" {
		return "", fmt.Errorf("网卡 GUID 为空，无法定位注册表项")
	}
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, classKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return "", fmt.Errorf("打开网卡类注册表项失败：%w", err)
	}
	defer root.Close()
	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return "", err
	}
	target := strings.Trim(guid, "{}")
	for _, name := range names {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, classKey+`\`+name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		id, _, err := key.GetStringValue("NetCfgInstanceId")
		key.Close()
		if err != nil {
			continue
		}
		if strings.EqualFold(strings.Trim(id, "{}"), target) {
			return name, nil
		}
	}
	return "", fmt.Errorf("未找到网卡的注册表参数项（GUID %s）", guid)
}

// restartAdapter disables then enables the adapter so the new MAC takes effect.
func restartAdapter(name string) error {
	if err := netcfg.SetAdapterEnabled(name, false); err != nil {
		return fmt.Errorf("禁用网卡失败（MAC 已写入，重启网卡后生效）：%w", err)
	}
	time.Sleep(400 * time.Millisecond)
	if err := netcfg.SetAdapterEnabled(name, true); err != nil {
		return fmt.Errorf("启用网卡失败（请在网络连接里手动启用）：%w", err)
	}
	return nil
}
