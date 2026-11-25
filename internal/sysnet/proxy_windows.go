//go:build windows

// Package sysnet manages Windows system proxy settings (HKCU Internet Settings
// + WinINet broadcast) and structured editing of the system hosts file.
package sysnet

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// ProxyConfig mirrors the Windows WinINet per-user proxy settings.
type ProxyConfig struct {
	Enabled  bool
	Server   string // e.g. "127.0.0.1:7890" or "http=1.2.3.4:8080;https=1.2.3.4:8080"
	Override string // semicolon-separated bypass list, e.g. "localhost;127.*;10.*;<local>"
}

// GetProxy reads the current Windows system proxy settings.
func GetProxy() (ProxyConfig, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		return ProxyConfig{}, fmt.Errorf("读取系统代理设置失败：%w", err)
	}
	defer key.Close()

	var cfg ProxyConfig
	if v, _, err := key.GetIntegerValue("ProxyEnable"); err == nil {
		cfg.Enabled = v != 0
	}
	if s, _, err := key.GetStringValue("ProxyServer"); err == nil {
		cfg.Server = s
	}
	if o, _, err := key.GetStringValue("ProxyOverride"); err == nil {
		cfg.Override = o
	}
	return cfg, nil
}

// SetProxy writes the Windows system proxy settings and notifies WinINet so
// browsers and applications pick up the change immediately.
func SetProxy(cfg ProxyConfig) error {
	server := strings.TrimSpace(cfg.Server)
	override := strings.TrimSpace(cfg.Override)
	if cfg.Enabled && server == "" {
		return fmt.Errorf("启用系统代理时必须填写代理服务器地址（如 127.0.0.1:7890）")
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开代理注册表项失败：%w", err)
	}
	defer key.Close()

	val := uint32(0)
	if cfg.Enabled {
		val = 1
	}
	if err := key.SetDWordValue("ProxyEnable", val); err != nil {
		return fmt.Errorf("写入 ProxyEnable 失败：%w", err)
	}
	if server != "" {
		if err := key.SetStringValue("ProxyServer", server); err != nil {
			return fmt.Errorf("写入 ProxyServer 失败：%w", err)
		}
	}
	if override != "" {
		if err := key.SetStringValue("ProxyOverride", override); err != nil {
			return fmt.Errorf("写入 ProxyOverride 失败：%w", err)
		}
	}
	notifyProxyChanged()
	return nil
}

func notifyProxyChanged() {
	wininet := windows.NewLazySystemDLL("wininet.dll")
	proc := wininet.NewProc("InternetSetOptionW")
	if proc.Find() != nil {
		return
	}
	const (
		internetOptionRefresh         = 37
		internetOptionSettingsChanged = 39
	)
	_, _, _ = proc.Call(0, internetOptionSettingsChanged, 0, 0)
	_, _, _ = proc.Call(0, internetOptionRefresh, 0, 0)
}
