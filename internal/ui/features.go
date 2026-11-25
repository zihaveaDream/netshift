package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"netshift/internal/hostsys"
	"netshift/internal/model"
	"netshift/internal/netcfg"
	"netshift/internal/netdiag"
	"netshift/internal/routing"
	"netshift/internal/store"
	"netshift/internal/sysnet"
)

// AdapterSnapshot captures an adapter's live IPv4 settings right before a
// modification so the user can roll back with one click.
type AdapterSnapshot struct {
	At      time.Time
	Request netcfg.Request
	Summary string
}

// ConflictState holds the IP conflict pre-check state and pending apply dialog.
type ConflictState struct {
	Enabled        bool // when true, applyNow runs ARP+ICMP conflict check before writing
	DialogOpen     bool
	Result         netdiag.ConflictResult
	PendingReq     netcfg.Request
	PendingProfile model.Profile
}

// DiagState holds state for the network connectivity diagnostic window.
type DiagState struct {
	DNSArg       string
	InternetArg  string
	PortHost     string
	PortNum      string
	Running      bool
	Report       netdiag.Report
	PortRunning  bool
	PortResults  []netdiag.CheckItem
}

// SysNetState holds state for the System Proxy & Hosts manager window.
type SysNetState struct {
	Tab           int
	Proxy         sysnet.ProxyConfig
	ProxyLoaded   bool
	ProxyErr      string
	Hosts         *sysnet.HostFile
	HostsQuery    string
	HostsErr      string
	NewHostIP     string
	NewHostDomain string
	NewHostNote   string
}

// captureSnapshot saves the adapter's current live IPv4 configuration before applying changes.
func (a *App) captureSnapshot(adapterName string) {
	if adapterName == "" {
		return
	}
	var ad netcfg.Adapter
	found := false
	for _, item := range a.adapters {
		if item.Name == adapterName {
			ad = item
			found = true
			break
		}
	}
	if !found {
		return
	}
	req := netcfg.Request{
		Adapter: ad.Name,
		DHCP:    ad.DHCP,
		IPv4:    append([]string(nil), ad.IPv4...),
		Masks:   append([]string(nil), ad.Masks...),
		DNS:     append([]string(nil), ad.DNS...),
	}
	for _, gw := range ad.Gateways {
		req.Gateways = append(req.Gateways, model.Gateway{Addr: gw})
	}
	summary := "DHCP 自动获取"
	if !ad.DHCP {
		if ip := ad.CurrentIP(); ip != "" {
			summary = ip
		} else {
			summary = "静态（无 IP）"
		}
	}
	if a.snapshots == nil {
		a.snapshots = map[string]AdapterSnapshot{}
	}
	a.snapshots[adapterName] = AdapterSnapshot{
		At:      time.Now(),
		Request: req,
		Summary: summary,
	}
}

func (a *App) currentSnapshot() (AdapterSnapshot, bool) {
	if a.snapshots == nil || a.adapter == "" {
		return AdapterSnapshot{}, false
	}
	s, ok := a.snapshots[a.adapter]
	return s, ok
}

// rollbackSnapshot restores the adapter to the configuration captured right
// before the last applied change.
func (a *App) rollbackSnapshot() {
	snap, ok := a.currentSnapshot()
	if !ok {
		a.setStatus("✗ 当前网卡暂无可回退的历史快照")
		return
	}
	req := snap.Request
	adapterName := req.Adapter
	site := a.profileLocation()
	sysName := a.Form.Computer
	profile := model.Profile{
		Name:     "回退快照",
		Location: site,
		Adapter:  adapterName,
		DHCP:     req.DHCP,
		IPv4:     append([]string(nil), req.IPv4...),
		Masks:    append([]string(nil), req.Masks...),
		Gateways: append([]model.Gateway(nil), req.Gateways...),
		DNS:      append([]string(nil), req.DNS...),
		Computer: sysName,
	}
	a.async(fmt.Sprintf("正在回退 [%s] 至 %s…", adapterName, snap.Summary), func() (string, error) {
		if _, err := netcfg.Apply(req); err != nil {
			return "", err
		}
		_ = a.store.AppendLog(store.LogFormat(time.Now(), site, sysName, adapterName, profile))
		time.Sleep(300 * time.Millisecond)
		list, _ := netcfg.List()
		host, _ := hostsys.Current()
		a.post(func() {
			a.applyRefreshedData(list, host, true)
			a.RefreshTrayMenu()
		})
		return fmt.Sprintf("已回退 [%s] 至 %s", adapterName, snap.Summary), nil
	})
}

// localMACSet returns all normalized MAC addresses belonging to this machine so
// conflict checks do not falsely flag the machine's own IP.
func (a *App) localMACSet() map[string]bool {
	m := make(map[string]bool, len(a.adapters))
	for _, ad := range a.adapters {
		if ad.MAC != "" {
			m[strings.ToUpper(ad.MAC)] = true
		}
	}
	return m
}

// checkConflictOnly manually probes the IP currently typed in the main form.
func (a *App) checkConflictOnly() {
	ip := strings.TrimSpace(a.Form.IP)
	if err := model.ValidateIPv4(ip); err != nil {
		a.setStatus("✗ 请先填写合法的 IPv4 地址")
		return
	}
	localMACs := a.localMACSet()
	a.async("正在检测 "+ip+" 是否冲突（ARP + ICMP）…", func() (string, error) {
		res := netdiag.CheckConflict(ip, localMACs, 1200*time.Millisecond)
		if res.Err != nil {
			return "", res.Err
		}
		if res.Occupied {
			if res.MAC != "" {
				return "", fmt.Errorf("检测到 IP 冲突：%s 已被设备 %s 占用（ARP）", ip, res.MAC)
			}
			return "", fmt.Errorf("检测到 IP 冲突：%s 有主机响应（ICMP %d ms）", ip, res.RTT.Milliseconds())
		}
		return fmt.Sprintf("%s 未检测到冲突，可安全使用", ip), nil
	})
}

// executeApplyReq performs the actual netsh apply (after snapshot capture).
func (a *App) executeApplyReq(req netcfg.Request, profile model.Profile) {
	adapterName := req.Adapter
	site, sysName := profile.Location, profile.Computer
	a.captureSnapshot(adapterName)
	a.async("正在应用配置到 ["+adapterName+"]…", func() (string, error) {
		if _, err := netcfg.Apply(req); err != nil {
			return "", err
		}
		_ = a.store.AppendLog(store.LogFormat(time.Now(), site, sysName, adapterName, profile))
		if req.DHCP {
			time.Sleep(400 * time.Millisecond)
		}
		list, _ := netcfg.List()
		host, _ := hostsys.Current()
		a.post(func() {
			a.applyRefreshedData(list, host, false)
			a.RefreshTrayMenu()
		})
		return "已应用配置，" + store.LogFile + " 已记录", nil
	})
}

// runBasicDiag runs gateway, DNS and internet reachability checks concurrently.
func (a *App) runBasicDiag() {
	if a.Diag.Running {
		return
	}
	ad := a.currentAdapter()
	gw := firstOr(ad.Gateways)
	if gw == "" {
		gw = strings.TrimSpace(a.Form.Gateway)
	}
	dnsHost := strings.TrimSpace(a.Diag.DNSArg)
	inetHost := strings.TrimSpace(a.Diag.InternetArg)
	a.Diag.Running = true
	a.setStatus("正在执行连通性诊断（网关 / DNS / 外网）…")
	go func() {
		report := netdiag.RunBasic(context.Background(), gw, dnsHost, inetHost)
		okCount := 0
		for _, it := range report.Items {
			if it.OK {
				okCount++
			}
		}
		a.post(func() {
			a.Diag.Running = false
			a.Diag.Report = report
			a.setStatus(fmt.Sprintf("✓ 连通性诊断完成：%d/%d 项正常", okCount, len(report.Items)))
		})
	}()
}

// runPortProbe tests a single TCP host:port and prepends the result to PortResults.
func (a *App) runPortProbe(host string, port int) {
	if a.Diag.PortRunning {
		return
	}
	host = strings.TrimSpace(host)
	if host == "" {
		a.setStatus("✗ 请填写目标主机或 IP")
		return
	}
	if port < 1 || port > 65535 {
		a.setStatus("✗ 端口号应在 1..65535 之间")
		return
	}
	a.Diag.PortRunning = true
	a.setStatus(fmt.Sprintf("正在探测 TCP %s:%d …", host, port))
	go func() {
		res := netdiag.ProbeTCP(context.Background(), host, port, 2*time.Second)
		a.post(func() {
			a.Diag.PortRunning = false
			a.Diag.PortResults = append([]netdiag.CheckItem{res}, a.Diag.PortResults...)
			if len(a.Diag.PortResults) > 12 {
				a.Diag.PortResults = a.Diag.PortResults[:12]
			}
			if res.OK {
				a.setStatus(fmt.Sprintf("✓ %s %s", res.Target, res.Detail))
			} else {
				a.setStatus(fmt.Sprintf("✗ %s %s", res.Target, res.Detail))
			}
		})
	}()
}

// deleteRouteEntry removes a single route entry and refreshes the routing table.
func (a *App) deleteRouteEntry(e routing.RouteEntry) {
	cmd, err := routing.DeleteCommand(e)
	if err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	a.async("正在删除路由："+cmd+"…", func() (string, error) {
		if _, err := routing.Delete(e); err != nil {
			return "", err
		}
		table, _ := routing.Table()
		entries := routing.ParseTable(table)
		a.post(func() {
			a.Route.Output = table
			a.Route.Entries = entries
		})
		return "已删除路由 " + e.Dest + "/" + e.Mask, nil
	})
}

// loadSysNet refreshes both system proxy settings and the hosts file.
func (a *App) loadSysNet() {
	if cfg, err := sysnet.GetProxy(); err != nil {
		a.SysNet.ProxyErr = err.Error()
	} else {
		a.SysNet.Proxy = cfg
		a.SysNet.ProxyLoaded = true
		a.SysNet.ProxyErr = ""
	}
	if hf, err := sysnet.LoadHosts(""); err != nil {
		a.SysNet.HostsErr = err.Error()
	} else {
		a.SysNet.Hosts = hf
		a.SysNet.HostsErr = ""
	}
}

func (a *App) saveProxy(cfg sysnet.ProxyConfig) {
	a.async("正在应用 Windows 系统代理设置…", func() (string, error) {
		if err := sysnet.SetProxy(cfg); err != nil {
			return "", err
		}
		fresh, _ := sysnet.GetProxy()
		a.post(func() {
			a.SysNet.Proxy = fresh
			a.SysNet.ProxyLoaded = true
			a.SysNet.ProxyErr = ""
		})
		if cfg.Enabled {
			return "系统代理已启用：" + cfg.Server, nil
		}
		return "系统代理已关闭并立即生效", nil
	})
}

func (a *App) saveHosts() {
	if a.SysNet.Hosts == nil {
		a.setStatus("✗ 未加载 hosts 文件")
		return
	}
	hf := a.SysNet.Hosts
	a.async("正在保存 hosts 并刷新 DNS 缓存…", func() (string, error) {
		if err := hf.Save(); err != nil {
			return "", err
		}
		_, _ = netcfg.FlushDNS()
		reloaded, _ := sysnet.LoadHosts(hf.Path)
		a.post(func() {
			if reloaded != nil {
				a.SysNet.Hosts = reloaded
			}
		})
		return "hosts 已保存并已刷新 DNS 解析缓存", nil
	})
}

func (a *App) addHostEntry() {
	if a.SysNet.Hosts == nil {
		a.SysNet.Hosts = sysnet.ParseHosts(sysnet.DefaultHostsPath(), "")
	}
	if err := a.SysNet.Hosts.Add(a.SysNet.NewHostIP, a.SysNet.NewHostDomain, a.SysNet.NewHostNote); err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	a.SysNet.NewHostIP = ""
	a.SysNet.NewHostDomain = ""
	a.SysNet.NewHostNote = ""
	a.setStatus("已添加到列表，点击「保存并刷新 DNS」生效")
}

// useSelectedPingIP copies the chosen host from the C-class ping grid into the
// main window's static IP field.
func (a *App) useSelectedPingIP() {
	n := a.Ping.SelectedHost
	if n < 1 || n > 254 {
		a.setStatus("✗ 请先在网格中点击选择一个主机号")
		return
	}
	prefix := strings.TrimSuffix(strings.TrimSpace(a.Ping.Prefix), ".")
	if prefix == "" {
		a.setStatus("✗ 网段前缀为空")
		return
	}
	ip := fmt.Sprintf("%s.%d", prefix, n)
	a.Form.Mode = 1
	a.Form.IP = ip
	if strings.TrimSpace(a.Form.Mask) == "" {
		a.Form.Mask = "255.255.255.0"
	}
	if strings.TrimSpace(a.Form.Gateway) == "" {
		a.Form.Gateway = prefix + ".1"
	}
	a.setStatus("✓ 已将 " + ip + " 填入主界面静态 IP")
}

func parsePortText(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
