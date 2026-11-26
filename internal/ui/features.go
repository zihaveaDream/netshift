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

// The waits below cover the delay between a netsh call and the state Windows
// reports afterwards: an adapter switch to DHCP releases the old address at once
// but only gets a lease after a few seconds.
const (
	dhcpSampleDelay = 400 * time.Millisecond // first look after switching to DHCP
	syncPollTimeout = 12 * time.Second       // how long a background wait follows an adapter
	syncPollEvery   = 500 * time.Millisecond
	staticPollLimit = 2500 * time.Millisecond // a static address shows up quickly
	staticPollEvery = 250 * time.Millisecond
	enableStartWait = 500 * time.Millisecond // an adapter restarts after enable/MAC change
	disablePollWait = 3 * time.Second
	disablePollRate = 250 * time.Millisecond
)

// cancelAdapterSync stops the background adapter wait and bumps the generation
// stamp, so a goroutine still running drops its late UI updates instead of
// painting over a newer sample.
func (a *App) cancelAdapterSync() {
	a.mu.Lock()
	a.syncGen++
	cancel := a.syncCancel
	a.syncCancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// startAdapterSync cancels the previous wait, arms a new timeout and returns the
// context together with the generation its updates must match.
func (a *App) startAdapterSync(timeout time.Duration) (context.Context, context.CancelFunc, uint64) {
	a.cancelAdapterSync()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	a.mu.Lock()
	a.syncCancel = cancel
	gen := a.syncGen
	a.mu.Unlock()
	return ctx, cancel, gen
}

// syncValid reports whether a background sample may still be shown: no newer
// sync started meanwhile and the main window is alive.
func (a *App) syncValid(gen uint64) bool {
	a.mu.Lock()
	current := a.syncGen == gen
	a.mu.Unlock()
	if !current {
		return false
	}
	return a.main == nil || !a.main.IsDestroyed()
}

// postAdapterRefresh re-reads the computer name and pushes a fresh adapter
// sample to the main window and the tray menu.
func (a *App) postAdapterRefresh(list []netcfg.Adapter, overwriteForm bool) {
	host, _ := hostsys.Current()
	if a.main == nil {
		a.applyRefreshedData(list, host, overwriteForm)
		return
	}
	a.post(func() {
		a.applyRefreshedData(list, host, overwriteForm)
		a.RefreshTrayMenu()
	})
}

// postAdapterRefreshIf pushes a fresh adapter sample only while gen is still
// the active sync generation, checking again on the UI thread to prevent stale
// queued callbacks from overwriting newer state.
func (a *App) postAdapterRefreshIf(gen uint64, list []netcfg.Adapter, overwriteForm bool) {
	if !a.syncValid(gen) {
		return
	}
	host, _ := hostsys.Current()
	if a.main == nil {
		if a.syncValid(gen) {
			a.applyRefreshedData(list, host, overwriteForm)
		}
		return
	}
	a.post(func() {
		if !a.syncValid(gen) {
			return
		}
		a.applyRefreshedData(list, host, overwriteForm)
		a.RefreshTrayMenu()
	})
}

// prevStaticIP returns the static address an adapter had before it was switched
// to DHCP, or "" when it was already automatic: the wait for a lease uses it to
// tell the freshly assigned address from the one that was just released.
func (a *App) prevStaticIP(adapterName string) string {
	for _, ad := range a.adapters {
		if ad.Name == adapterName {
			if ad.DHCP {
				return ""
			}
			return ad.CurrentIP()
		}
	}
	return ""
}

// adapterByName finds an adapter in a freshly listed snapshot.
func adapterByName(list []netcfg.Adapter, name string) (netcfg.Adapter, bool) {
	for _, ad := range list {
		if ad.Name == name {
			return ad, true
		}
	}
	return netcfg.Adapter{}, false
}

// containsAddress reports whether list holds ip.
func containsAddress(list []string, ip string) bool {
	for _, v := range list {
		if strings.TrimSpace(v) == ip {
			return true
		}
	}
	return false
}

// syncAdapterDHCP refreshes the state of an adapter that was just switched to
// automatic addressing, and when the lease is still pending follows the
// assignment in the background so the dropdown, the badge and the form stop
// showing the address that was released.
//
// It reports the status itself and must not hold the busy flag while waiting: the
// user has to be able to press 刷新数据 at any time.
func (a *App) syncAdapterDHCP(adapterName, prevStaticIP string, overwriteForm bool) {
	a.cancelAdapterSync()
	time.Sleep(dhcpSampleDelay)
	list, _ := netcfg.List()
	host, _ := hostsys.Current()
	ad, found := adapterByName(list, adapterName)
	status := ""
	pending := false
	switch {
	case !found:
		status = "已切换为自动获取，未检测到网卡 [" + adapterName + "]"
	case !ad.Up:
		status = "已切换为自动获取（当前网卡未连接）"
	case ad.DHCP && ad.HasUsableIPv4() && (prevStaticIP == "" || ad.UsableIPv4() != prevStaticIP):
		status = fmt.Sprintf("✓ [%s] 已恢复自动获取，当前 IP：%s", adapterName, ad.UsableIPv4())
	default:
		// No address yet, an APIPA one, or the old static address still listed.
		pending = true
		status = "已切换为自动获取，正在等待 DHCP 分配 IP…"
	}
	if a.main == nil {
		a.applyRefreshedData(list, host, overwriteForm)
		a.setStatus(status)
		return
	}
	a.post(func() {
		a.applyRefreshedData(list, host, overwriteForm)
		a.RefreshTrayMenu()
		a.setStatus(status)
	})
	if !pending {
		return
	}

	initialCleared := !found || !ad.DHCP || !ad.HasUsableIPv4() ||
		(prevStaticIP != "" && ad.UsableIPv4() != prevStaticIP)
	ctx, cancel, gen := a.startAdapterSync(syncPollTimeout)
	go func() {
		defer cancel()
		start := time.Now()
		sawCleared := initialCleared
		pred := func(ad netcfg.Adapter) bool {
			if !ad.Up {
				// The cable was pulled while waiting: stop instead of polling on.
				return true
			}
			if !ad.DHCP || !ad.HasUsableIPv4() {
				sawCleared = true
				return false
			}
			if ip := ad.UsableIPv4(); prevStaticIP == "" || ip != prevStaticIP {
				return true
			}
			// Still the address the adapter had as a static one: ready once it was
			// released at some point, or once it is clear DHCP handed the same one back.
			return sawCleared || time.Since(start) >= staticPollLimit
		}
		list, ad, ok := netcfg.AwaitAdapter(ctx, adapterName, syncPollEvery, pred,
			func(list []netcfg.Adapter, _ netcfg.Adapter) {
				a.postAdapterRefreshIf(gen, list, overwriteForm)
			})
		if !a.syncValid(gen) {
			return
		}
		host, _ := hostsys.Current()
		a.post(func() {
			if !a.syncValid(gen) {
				return
			}
			a.applyRefreshedData(list, host, overwriteForm)
			a.RefreshTrayMenu()
			switch {
			case !ad.Up:
				a.setStatus("已切换为自动获取（当前网卡未连接）")
			case ok && ad.HasUsableIPv4():
				a.setStatus(fmt.Sprintf("✓ [%s] 已自动获取 IP：%s", adapterName, ad.UsableIPv4()))
			case netcfg.IsAPIPA(ad.CurrentIP()):
				a.setStatus("⚠ [" + adapterName + "] 已切换为自动获取，仅获取到自动专用地址（169.254.x.x），请检查 DHCP 服务或稍后点击「刷新数据」")
			default:
				a.setStatus("⚠ [" + adapterName + "] 已切换为自动获取，暂未获取到 IP，请检查网络或稍后点击「刷新数据」")
			}
		})
	}()
}

// syncAdapterOnline refreshes the UI after an adapter was enabled or restarted
// (MAC change) and keeps following it until it is up with a usable address, so
// the dropdown, the badge and the form pick the new address up by themselves.
func (a *App) syncAdapterOnline(adapterName string) {
	a.cancelAdapterSync()
	time.Sleep(enableStartWait)
	list, _ := netcfg.List()
	a.postAdapterRefresh(list, false)
	if a.main == nil {
		return
	}
	if ad, ok := adapterByName(list, adapterName); ok && ad.Up && (!ad.DHCP || ad.HasUsableIPv4()) {
		return
	}

	ctx, cancel, gen := a.startAdapterSync(syncPollTimeout)
	go func() {
		defer cancel()
		list, ad, ok := netcfg.AwaitAdapter(ctx, adapterName, syncPollEvery,
			func(ad netcfg.Adapter) bool { return ad.Up && (!ad.DHCP || ad.HasUsableIPv4()) },
			func(list []netcfg.Adapter, _ netcfg.Adapter) {
				a.postAdapterRefreshIf(gen, list, false)
			})
		if !a.syncValid(gen) {
			return
		}
		host, _ := hostsys.Current()
		a.post(func() {
			if !a.syncValid(gen) {
				return
			}
			a.applyRefreshedData(list, host, false)
			a.RefreshTrayMenu()
			switch {
			case ok && ad.HasUsableIPv4():
				a.setStatus(fmt.Sprintf("✓ [%s] 已获取 IP：%s", adapterName, ad.UsableIPv4()))
			case ok && ad.Up:
				a.setStatus("✓ [" + adapterName + "] 网卡已启用")
			default:
				a.setStatus("⚠ [" + adapterName + "] 网卡已启用，暂未获取到 IP，请稍后点击「刷新数据」")
			}
		})
	}()
}

// syncAdapterDisabled polls briefly until a disabled adapter reports down, then
// refreshes the UI.
func (a *App) syncAdapterDisabled(adapterName string) {
	ctx, cancel, gen := a.startAdapterSync(disablePollWait)
	defer cancel()
	list, _, _ := netcfg.AwaitAdapter(ctx, adapterName, disablePollRate,
		func(ad netcfg.Adapter) bool { return !ad.Up },
		func(list []netcfg.Adapter, _ netcfg.Adapter) {
			a.postAdapterRefreshIf(gen, list, false)
		})
	if len(list) == 0 {
		list, _ = netcfg.List()
	}
	a.postAdapterRefreshIf(gen, list, false)
}

// waitAdapterStatic waits out the short delay before a just-applied static
// address shows up in the adapter list, refreshing the UI as soon as it does so
// neither the form nor the badge keeps showing the replaced address.
func (a *App) waitAdapterStatic(adapterName, targetIP string, overwriteForm bool) {
	targetIP = strings.TrimSpace(targetIP)
	if targetIP == "" {
		a.cancelAdapterSync()
		list, _ := netcfg.List()
		a.postAdapterRefresh(list, overwriteForm)
		return
	}
	ctx, cancel, gen := a.startAdapterSync(staticPollLimit)
	defer cancel()
	list, _, _ := netcfg.AwaitAdapter(ctx, adapterName, staticPollEvery,
		func(ad netcfg.Adapter) bool { return !ad.DHCP && containsAddress(ad.IPv4, targetIP) },
		func(list []netcfg.Adapter, _ netcfg.Adapter) {
			a.postAdapterRefreshIf(gen, list, overwriteForm)
		})
	if len(list) == 0 {
		return
	}
	a.postAdapterRefreshIf(gen, list, overwriteForm)
}

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
	prevStatic := ""
	if req.DHCP {
		prevStatic = a.prevStaticIP(adapterName)
	}
	targetIP := ""
	if !req.DHCP && len(req.IPv4) > 0 {
		targetIP = req.IPv4[0]
	}
	a.async(fmt.Sprintf("正在回退 [%s] 至 %s…", adapterName, snap.Summary), func() (string, error) {
		if _, err := netcfg.Apply(req); err != nil {
			return "", err
		}
		_ = a.store.AppendLog(store.LogFormat(time.Now(), site, sysName, adapterName, profile))
		if req.DHCP {
			a.syncAdapterDHCP(adapterName, prevStatic, true)
			return "", nil
		}
		a.waitAdapterStatic(adapterName, targetIP, true)
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
	prevStatic := ""
	if req.DHCP {
		prevStatic = a.prevStaticIP(adapterName)
	}
	targetIP := ""
	if !req.DHCP && len(req.IPv4) > 0 {
		targetIP = req.IPv4[0]
	}
	a.captureSnapshot(adapterName)
	a.async("正在应用配置到 ["+adapterName+"]…", func() (string, error) {
		if _, err := netcfg.Apply(req); err != nil {
			return "", err
		}
		_ = a.store.AppendLog(store.LogFormat(time.Now(), site, sysName, adapterName, profile))
		if req.DHCP {
			// The lease arrives seconds after netsh returns: report and follow it
			// instead of reading the adapter once and showing an empty address.
			a.syncAdapterDHCP(adapterName, prevStatic, false)
			return "", nil
		}
		a.waitAdapterStatic(adapterName, targetIP, false)
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
