// Package ui draws the app and runs its actions: the main window, the advanced
// options, the presets, and the toolbox screens.
package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"netshift/internal/hostsys"
	"netshift/internal/macaddr"
	"netshift/internal/model"
	"netshift/internal/netcfg"
	"netshift/internal/netdiag"
	"netshift/internal/pingfan"
	"netshift/internal/routing"
	"netshift/internal/store"
	"netshift/internal/subnet"
)

// Form is what the main window's controls hold.
type Form struct {
	Mode      int // 0: obtain automatically, 1: use the addresses below
	IP        string
	Mask      string
	Gateway   string
	DNS1      string
	DNS2      string
	Computer  string
	Workgroup string
	MAC       string
}

// Profile turns the form into a profile for adapter.
func (f Form) Profile(name, location, adapter string) model.Profile {
	p := model.Profile{Name: name, Location: location, Adapter: adapter}
	p.DHCP = f.Mode == 0
	if !p.DHCP {
		if v := strings.TrimSpace(f.IP); v != "" {
			p.IPv4 = []string{v}
		}
		if v := strings.TrimSpace(f.Mask); v != "" {
			p.Masks = []string{v}
		}
		if v := strings.TrimSpace(f.Gateway); v != "" {
			p.Gateways = []model.Gateway{{Addr: v}}
		}
	}
	if v := strings.TrimSpace(f.DNS1); v != "" {
		p.DNS = append(p.DNS, v)
	}
	if v := strings.TrimSpace(f.DNS2); v != "" {
		p.DNS = append(p.DNS, v)
	}
	p.Computer = strings.TrimSpace(f.Computer)
	p.Workgroup = strings.TrimSpace(f.Workgroup)
	p.MAC = strings.TrimSpace(f.MAC)
	return p
}

// LoadFrom fills the form from a profile.
func (f *Form) LoadFrom(p model.Profile) {
	if p.DHCP {
		f.Mode = 0
	} else {
		f.Mode = 1
	}
	f.IP = p.PrimaryIP()
	f.Mask = p.PrimaryMask()
	f.Gateway = p.PrimaryGateway()
	f.DNS1 = p.PrimaryDNS()
	f.DNS2 = p.SecondaryDNS()
	f.Computer = p.Computer
	f.Workgroup = p.Workgroup
	f.MAC = p.MAC
}

// LoadFromAdapter fills the form with what the adapter has now.
func (f *Form) LoadFromAdapter(a netcfg.Adapter, host hostsys.Info) {
	if a.DHCP {
		f.Mode = 0
	} else {
		f.Mode = 1
	}
	f.IP = a.CurrentIP()
	f.Mask = ""
	if len(a.Masks) > 0 {
		f.Mask = a.Masks[0]
	}
	f.Gateway = ""
	if len(a.Gateways) > 0 {
		f.Gateway = a.Gateways[0]
	}
	f.DNS1, f.DNS2 = "", ""
	if len(a.DNS) > 0 {
		f.DNS1 = a.DNS[0]
	}
	if len(a.DNS) > 1 {
		f.DNS2 = a.DNS[1]
	}
	f.Computer = host.Name
	f.Workgroup = host.Workgroup
	f.MAC = macaddr.Format(a.MAC)
}

// AddressRow is one editable entry of the advanced options tables.
type AddressRow struct {
	Value  string // IP, gateway or DNS server
	Mask   string // subnet mask, for the IP table
	Metric string // metric, for the gateway table
}

// Advanced is the state of the advanced options window.
type Advanced struct {
	Tab          int
	LastTab      int
	IPs          []AddressRow
	GWs          []AddressRow
	DNS          []AddressRow
	Draft        AddressRow
	MetricAuto   bool
	MetricCustom string
	PageOnly     bool
}

// Presets is the state of the presets window.
type Presets struct {
	Query      string
	Selected   int
	EditingOld string // original name of the selected profile, for in-place rename
	Pick       int
	Name       string
	Location   string
	Edit       Form
	Dirty      bool
}

// SubnetState is the subnet calculator's state.
type SubnetState struct {
	IP         string
	Prefix     float64
	PrefixText string
	Manual     bool
	Info       subnet.Info
	Err        string
}

// PingState is the C-class ping screen's state.
type PingState struct {
	Prefix       string
	Workers      int
	WorkersText  string
	Timeout      float64 // milliseconds
	TimeoutText  string
	State        [256]int // 0 unknown, 1 online, 2 offline
	RTT          [256]int
	Running      bool
	Done         int
	Online       int
	SelectedHost int // last clicked host octet (1..254)
	cancel       context.CancelFunc
}

// RouteState is the split-routing screen's state.
type RouteState struct {
	Tab            int // 0: split-route config, 1: IPv4 route table manager
	Adapter        string
	Net             string
	Mask            string
	Gateway         string
	NetChoice       string
	MaskChoice      string
	KeepDefault     bool
	Persist         bool
	Preview         []string
	Output          string
	Entries         []routing.RouteEntry
	Filter          string
	OnlyPersistent  bool
	ShowRawOutput   bool
	Err             string
	Done            bool
}

// App is the whole application state.
type App struct {
	store *store.Store

	adapters     []netcfg.Adapter
	adaptersErr  string
	adapter      string
	adapterLabel string // value bound to the adapter select
	host         hostsys.Info
	hostErr      string
	profiles     []model.Profile
	profilesErr  string
	elevated     bool

	Form     Form
	Adv      Advanced
	Pre      Presets
	Sub      SubnetState
	Ping     PingState
	Route    RouteState
	Conflict ConflictState
	Diag     DiagState
	SysNet   SysNetState

	snapshots map[string]AdapterSnapshot

	LogText        string
	ConfirmRestart bool

	status  string
	version string
	busy    int
	mu      sync.Mutex

	main   *mygo.Window
	tray   *mygo.Tray
	wins   map[string]*mygo.Window
	loaded bool
}

// New prepares the app: it reads the adapters, the computer name and the saved
// profiles, and imports the batch script's history once.
func New(dir, version string) *App {
	a := &App{
		store:     store.New(dir),
		version:   version,
		elevated:  netcfg.IsElevated(),
		wins:      map[string]*mygo.Window{},
		snapshots: map[string]AdapterSnapshot{},
		Adv:       Advanced{MetricAuto: true, MetricCustom: "10"},
		Ping:      PingState{Workers: 32, WorkersText: "32", Timeout: 1000, TimeoutText: "1000"},
		Sub:       SubnetState{Prefix: 24, PrefixText: "24"},
		Conflict:  ConflictState{Enabled: true},
		Diag:      DiagState{DNSArg: "www.baidu.com", InternetArg: "223.5.5.5", PortHost: "192.168.1.1", PortNum: "80"},
	}
	a.load()
	return a
}

func (a *App) load() {
	profiles, err := a.store.Load()
	a.profiles = profiles
	if err != nil {
		a.profilesErr = err.Error()
	}
	if err == nil && len(a.profiles) == 0 && a.store.LegacyExists() {
		if imported, err := a.store.ImportLegacy(); err == nil && len(imported) > 0 {
			a.profiles = append(a.profiles, imported...)
			_ = a.store.Save(a.profiles)
			_ = a.store.BackupLegacy(time.Now())
			a.status = fmt.Sprintf("✓ 已从 ip_history.txt 导入 %d 条历史配置", len(imported))
		}
	}
	a.refreshAdapters()
	a.refreshHost()
	if ad, ok := a.currentAdapterOK(); ok {
		a.Form.LoadFromAdapter(ad, a.host)
		a.loadAdvancedFromAdapter(ad)
	} else {
		a.loadAdvancedFromForm()
	}
	a.adapterLabel = a.adapterChoice()
	a.Sub.IP = a.Form.IP
	if a.Sub.IP == "" {
		a.Sub.IP = "192.168.1.10"
	}
	if p := model.MaskPrefix(a.Form.Mask); p >= 0 {
		a.Sub.Prefix = float64(p)
		a.Sub.PrefixText = strconv.Itoa(p)
	}
	a.recalcSubnet()
	if a.status == "" {
		if a.profilesErr != "" {
			a.status = "✗ " + a.profilesErr
		} else {
			a.status = "就绪"
		}
	}
	a.loaded = true
}

// Store returns the app's file store.
func (a *App) Store() *store.Store { return a.store }

// SetMainWindow tells the app which window to post background results to.
func (a *App) SetMainWindow(w *mygo.Window) { a.main = w }

// Version returns the version string shown in the status bar.
func (a *App) Version() string { return a.version }

// IdealMainSize calculates the recommended main window width & height in DIPs
// so that the longest detected network adapter label fits completely in the
// dropdown without ellipsis truncation.
func (a *App) IdealMainSize() (int, int) {
	const (
		minWantW = 1020
		wantH    = 620
	)
	var maxTextPx float64
	for _, ad := range a.adapters {
		w := estimateLabelWidthDIP(ad.Label())
		if w > maxTextPx {
			maxTextPx = w
		}
	}
	// Left panel inner chrome: panelPad*2 (26) + labelW (88) + gaps (18) +
	// select padding/chevron (56) + refresh button (92) = 280 DIP.
	// Outer window chrome + right panel: windowPad*2 (28) + colGap (14) + rightCol (374) = 416 DIP.
	neededW := int(maxTextPx) + 280 + 416
	if neededW < minWantW {
		neededW = minWantW
	}
	return neededW, wantH
}

func estimateLabelWidthDIP(s string) float64 {
	var px float64
	for _, r := range s {
		if r > 0x7f {
			px += 14.5
		} else if r >= 'A' && r <= 'Z' || r == 'm' || r == 'w' {
			px += 9.5
		} else {
			px += 7.8
		}
	}
	return px
}

// refreshAdapters re-reads the adapter list and keeps the selection.
func (a *App) refreshAdapters() {
	list, err := netcfg.List()
	if err != nil {
		a.adaptersErr = err.Error()
		return
	}
	a.applyAdapters(list)
}

func (a *App) applyAdapters(list []netcfg.Adapter) {
	a.adaptersErr = ""
	a.adapters = list
	kept := false
	if a.adapter != "" {
		for _, ad := range list {
			if ad.Name == a.adapter {
				kept = true
				break
			}
		}
	}
	if !kept {
		if len(list) > 0 {
			a.adapter = list[0].Name
		} else {
			a.adapter = ""
		}
	}
	a.adapterLabel = a.adapterChoice()
}

func (a *App) refreshHost() {
	info, err := hostsys.Current()
	if err != nil {
		a.hostErr = err.Error()
		return
	}
	a.hostErr = ""
	a.host = info
}

// currentAdapter returns the selected adapter from the in-memory cache, or zero value.
// Never performs Win32/registry I/O so it is safe to call every frame.
func (a *App) currentAdapter() netcfg.Adapter {
	ad, _ := a.currentAdapterOK()
	return ad
}

func (a *App) currentAdapterOK() (netcfg.Adapter, bool) {
	for _, ad := range a.adapters {
		if ad.Name == a.adapter {
			return ad, true
		}
	}
	return netcfg.Adapter{}, false
}

// adapterNames returns the labels shown in the adapter select.
func (a *App) adapterNames() []string {
	out := make([]string, 0, len(a.adapters))
	for _, ad := range a.adapters {
		out = append(out, ad.Label())
	}
	return out
}

// adapterChoice returns the label of the selected adapter from cached list.
func (a *App) adapterChoice() string {
	if ad, ok := a.currentAdapterOK(); ok {
		return ad.Label()
	}
	return a.adapter
}

// selectAdapterByLabel sets the selection from a label the select returned.
func (a *App) selectAdapterByLabel(label string) {
	for _, ad := range a.adapters {
		if ad.Label() == label {
			if ad.Name != a.adapter {
				a.adapter = ad.Name
				a.adapterLabel = ad.Label()
				a.Form.LoadFromAdapter(ad, a.host)
				a.loadAdvancedFromAdapter(ad)
				a.Sub.IP = ad.CurrentIP()
				if a.Sub.IP == "" {
					a.Sub.IP = "192.168.1.10"
				}
				if p := model.MaskPrefix(a.Form.Mask); p >= 0 {
					a.Sub.Prefix = float64(p)
					a.Sub.PrefixText = strconv.Itoa(p)
				}
				a.recalcSubnet()
				a.setStatus("已切换网卡：" + ad.Name)
			}
			return
		}
	}
}

// loadAdvancedFromAdapter seeds the advanced tables with all live addresses of ad.
func (a *App) loadAdvancedFromAdapter(ad netcfg.Adapter) {
	a.Adv.IPs = nil
	a.Adv.GWs = nil
	a.Adv.DNS = nil
	for i, ip := range ad.IPv4 {
		mask := ""
		if i < len(ad.Masks) {
			mask = ad.Masks[i]
		}
		a.Adv.IPs = append(a.Adv.IPs, AddressRow{Value: ip, Mask: mask})
	}
	for _, gw := range ad.Gateways {
		a.Adv.GWs = append(a.Adv.GWs, AddressRow{Value: gw})
	}
	for _, d := range ad.DNS {
		a.Adv.DNS = append(a.Adv.DNS, AddressRow{Value: d})
	}
}

// loadAdvancedFromForm seeds the advanced tables from the main form.
func (a *App) loadAdvancedFromForm() {
	a.Adv.IPs = nil
	a.Adv.GWs = nil
	a.Adv.DNS = nil
	if strings.TrimSpace(a.Form.IP) != "" {
		a.Adv.IPs = append(a.Adv.IPs, AddressRow{Value: strings.TrimSpace(a.Form.IP), Mask: strings.TrimSpace(a.Form.Mask)})
	}
	if strings.TrimSpace(a.Form.Gateway) != "" {
		a.Adv.GWs = append(a.Adv.GWs, AddressRow{Value: strings.TrimSpace(a.Form.Gateway)})
	}
	if strings.TrimSpace(a.Form.DNS1) != "" {
		a.Adv.DNS = append(a.Adv.DNS, AddressRow{Value: strings.TrimSpace(a.Form.DNS1)})
	}
	if strings.TrimSpace(a.Form.DNS2) != "" {
		a.Adv.DNS = append(a.Adv.DNS, AddressRow{Value: strings.TrimSpace(a.Form.DNS2)})
	}
}

// syncFormToAdvanced keeps the primary entries of Adv aligned with Form while
// preserving any extra IPs, gateways or DNS servers configured in Advanced.
func (a *App) syncFormToAdvanced() {
	ip := strings.TrimSpace(a.Form.IP)
	mask := strings.TrimSpace(a.Form.Mask)
	if ip != "" {
		if len(a.Adv.IPs) == 0 {
			a.Adv.IPs = []AddressRow{{Value: ip, Mask: mask}}
		} else {
			a.Adv.IPs[0].Value = ip
			a.Adv.IPs[0].Mask = mask
		}
	} else if len(a.Adv.IPs) == 1 {
		a.Adv.IPs = nil
	}

	gw := strings.TrimSpace(a.Form.Gateway)
	if gw != "" {
		if len(a.Adv.GWs) == 0 {
			a.Adv.GWs = []AddressRow{{Value: gw}}
		} else {
			a.Adv.GWs[0].Value = gw
		}
	} else if len(a.Adv.GWs) == 1 {
		a.Adv.GWs = nil
	}

	d1 := strings.TrimSpace(a.Form.DNS1)
	d2 := strings.TrimSpace(a.Form.DNS2)
	var extraDNS []AddressRow
	if len(a.Adv.DNS) > 2 {
		extraDNS = append(extraDNS, a.Adv.DNS[2:]...)
	}
	var nextDNS []AddressRow
	if d1 != "" {
		nextDNS = append(nextDNS, AddressRow{Value: d1})
	}
	if d2 != "" {
		nextDNS = append(nextDNS, AddressRow{Value: d2})
	}
	nextDNS = append(nextDNS, extraDNS...)
	a.Adv.DNS = nextDNS
}

// formFromAdvanced writes the advanced tables back into the main form.
func (a *App) formFromAdvanced() {
	if len(a.Adv.IPs) > 0 {
		a.Form.IP = a.Adv.IPs[0].Value
		a.Form.Mask = a.Adv.IPs[0].Mask
	} else {
		a.Form.IP, a.Form.Mask = "", ""
	}
	if len(a.Adv.GWs) > 0 {
		a.Form.Gateway = a.Adv.GWs[0].Value
	} else {
		a.Form.Gateway = ""
	}
	a.Form.DNS1, a.Form.DNS2 = "", ""
	if len(a.Adv.DNS) > 0 {
		a.Form.DNS1 = a.Adv.DNS[0].Value
	}
	if len(a.Adv.DNS) > 1 {
		a.Form.DNS2 = a.Adv.DNS[1].Value
	}
}

// request builds the netcfg request from Form + extra entries in Adv.
func (a *App) request() (netcfg.Request, error) {
	if a.adapter == "" {
		return netcfg.Request{}, fmt.Errorf("没有可用的网卡")
	}
	a.syncFormToAdvanced()
	req := netcfg.Request{Adapter: a.adapter, DHCP: a.Form.Mode == 0}
	if !req.DHCP {
		for _, row := range a.Adv.IPs {
			v := strings.TrimSpace(row.Value)
			if v == "" {
				continue
			}
			req.IPv4 = append(req.IPv4, v)
			req.Masks = append(req.Masks, strings.TrimSpace(row.Mask))
		}
		for _, g := range a.Adv.GWs {
			v := strings.TrimSpace(g.Value)
			if v == "" {
				continue
			}
			m := 0
			if ms := strings.TrimSpace(g.Metric); ms != "" {
				parsed, err := strconv.Atoi(ms)
				if err != nil || parsed < 0 || parsed > 9999 {
					return netcfg.Request{}, fmt.Errorf("网关 %s 的跃点数无效：%s", v, ms)
				}
				m = parsed
			}
			req.Gateways = append(req.Gateways, model.Gateway{Addr: v, Metric: m})
		}
		for _, d := range a.Adv.DNS {
			if v := strings.TrimSpace(d.Value); v != "" {
				req.DNS = append(req.DNS, v)
			}
		}
	}
	return req, nil
}

// buildFullProfile builds a complete Profile (including multi-IP/GW/DNS from Adv).
func (a *App) buildFullProfile(name, location string) model.Profile {
	a.syncFormToAdvanced()
	p := a.Form.Profile(name, location, a.adapter)
	if !p.DHCP && len(a.Adv.IPs) > 0 {
		p.IPv4 = nil
		p.Masks = nil
		for _, row := range a.Adv.IPs {
			if v := strings.TrimSpace(row.Value); v != "" {
				p.IPv4 = append(p.IPv4, v)
				p.Masks = append(p.Masks, strings.TrimSpace(row.Mask))
			}
		}
	}
	if !p.DHCP && len(a.Adv.GWs) > 0 {
		p.Gateways = nil
		for _, g := range a.Adv.GWs {
			if v := strings.TrimSpace(g.Value); v != "" {
				m, _ := strconv.Atoi(strings.TrimSpace(g.Metric))
				p.Gateways = append(p.Gateways, model.Gateway{Addr: v, Metric: m})
			}
		}
	}
	if p.DHCP {
		p.DNS = nil
	} else if len(a.Adv.DNS) > 0 {
		p.DNS = nil
		for _, d := range a.Adv.DNS {
			if v := strings.TrimSpace(d.Value); v != "" {
				p.DNS = append(p.DNS, v)
			}
		}
	}
	return p
}

// post runs fn on the main UI thread via Window.Update and invalidates all open
// child windows so every view reflects state changes immediately.
func (a *App) post(fn func()) {
	if a.main == nil {
		return
	}
	a.main.Update(func() {
		fn()
		a.invalidateChildren()
	})
}

func (a *App) invalidateChildren() {
	for _, w := range a.wins {
		if w != nil && !w.IsDestroyed() {
			w.Invalidate()
		}
	}
}

func (a *App) invalidateAll() {
	if a.main != nil && !a.main.IsDestroyed() {
		a.main.Invalidate()
	}
	a.invalidateChildren()
}

// async runs work off the UI thread and reports back on the main one.
func (a *App) async(label string, work func() (string, error)) {
	a.mu.Lock()
	a.busy++
	a.status = label
	a.mu.Unlock()
	a.invalidateAll()
	go func() {
		msg, err := work()
		if a.main == nil {
			a.mu.Lock()
			a.busy--
			if err != nil {
				a.status = "✗ " + err.Error()
			} else if msg != "" {
				a.status = "✓ " + msg
			}
			a.mu.Unlock()
			return
		}
		a.post(func() {
			a.mu.Lock()
			a.busy--
			if err != nil {
				a.status = "✗ " + err.Error()
			} else if msg != "" {
				a.status = "✓ " + msg
			}
			a.mu.Unlock()
		})
	}()
}

// busyNow reports whether a job is running.
func (a *App) busyNow() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.busy > 0
}

func (a *App) setStatus(s string) {
	a.mu.Lock()
	a.status = s
	a.mu.Unlock()
	a.invalidateAll()
}

// statusText returns the status bar text.
func (a *App) statusText() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

// applyNow applies the form's configuration to the selected adapter, optionally
// running an ARP+ICMP IP conflict check first when configuring a static IP.
func (a *App) applyNow() {
	req, err := a.request()
	if err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	profile := a.buildFullProfile(a.profileName(), a.profileLocation())
	if err := profile.Validate(); err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	if !req.DHCP && a.Conflict.Enabled && len(req.IPv4) > 0 && a.main != nil {
		targetIP := req.IPv4[0]
		localMACs := a.localMACSet()
		a.async("正在预检 "+targetIP+" 是否存在 IP 冲突…", func() (string, error) {
			res := netdiag.CheckConflict(targetIP, localMACs, 1000*time.Millisecond)
			if res.Occupied {
				a.post(func() {
					a.Conflict.Result = res
					a.Conflict.PendingReq = req
					a.Conflict.PendingProfile = profile
					a.Conflict.DialogOpen = true
				})
				return "", fmt.Errorf("发现 IP 冲突（%s），请确认是否继续", targetIP)
			}
			a.post(func() { a.executeApplyReq(req, profile) })
			return "未检测到冲突，正在写入配置…", nil
		})
		return
	}
	a.executeApplyReq(req, profile)
}

// profileName is the name a saved profile gets by default.
func (a *App) profileName() string {
	if strings.TrimSpace(a.Pre.Name) != "" {
		return strings.TrimSpace(a.Pre.Name)
	}
	if strings.TrimSpace(a.Form.Computer) != "" && a.adapter != "" {
		return a.adapter + "-" + strings.TrimSpace(a.Form.Computer)
	}
	if a.adapter != "" {
		return a.adapter
	}
	return "默认方案"
}

func (a *App) profileLocation() string {
	return a.Pre.Location
}

// applyDHCP switches the adapter back to automatic addressing.
func (a *App) applyDHCP() {
	adapterName := a.adapter
	if adapterName == "" {
		a.setStatus("✗ 没有可用的网卡")
		return
	}
	a.captureSnapshot(adapterName)
	a.Form.Mode = 0
	req := netcfg.Request{Adapter: adapterName, DHCP: true}
	site := a.profileLocation()
	sysName := a.Form.Computer
	profile := a.Form.Profile(a.profileName(), site, adapterName)
	a.async("正在恢复 ["+adapterName+"] 自动获取 IP 和 DNS…", func() (string, error) {
		if _, err := netcfg.Apply(req); err != nil {
			return "", err
		}
		_ = a.store.AppendLog(store.LogFormat(time.Now(), site, sysName, adapterName, profile))
		time.Sleep(400 * time.Millisecond)
		list, _ := netcfg.List()
		host, _ := hostsys.Current()
		a.post(func() {
			a.applyRefreshedData(list, host, false)
			a.RefreshTrayMenu()
		})
		return "已恢复自动获取 IP 和 DNS", nil
	})
}

// applyRefreshedData updates cached adapters/host on the UI thread.
// When overwriteForm is true (explicit Refresh button), all form fields are
// reloaded from live adapter values; otherwise static edits in progress are kept.
func (a *App) applyRefreshedData(list []netcfg.Adapter, host hostsys.Info, overwriteForm bool) {
	if len(list) > 0 {
		a.applyAdapters(list)
	}
	if host.Name != "" {
		a.host = host
		a.hostErr = ""
	}
	ad, ok := a.currentAdapterOK()
	if !ok {
		return
	}
	if overwriteForm {
		a.Form.LoadFromAdapter(ad, a.host)
		a.loadAdvancedFromAdapter(ad)
		a.Sub.IP = ad.CurrentIP()
		if a.Sub.IP == "" {
			a.Sub.IP = "192.168.1.10"
		}
		a.recalcSubnet()
		return
	}
	keepDHCP := a.Form.Mode == 0
	keep := a.Form
	loaded := Form{}
	loaded.LoadFromAdapter(ad, a.host)
	if keepDHCP {
		a.Form.Mode = loaded.Mode
	}
	a.Form.Computer = loaded.Computer
	a.Form.Workgroup = loaded.Workgroup
	a.Form.MAC = loaded.MAC
	if keep.IP == "" || keepDHCP {
		a.Form.IP = loaded.IP
		a.Form.Mask = loaded.Mask
		a.Form.Gateway = loaded.Gateway
	}
	if keep.DNS1 == "" || keepDHCP {
		a.Form.DNS1 = loaded.DNS1
		a.Form.DNS2 = loaded.DNS2
	}
}

// toggleAdapter enables or disables the selected adapter.
func (a *App) toggleAdapter(on bool) {
	adapterName := a.adapter
	if adapterName == "" {
		a.setStatus("✗ 没有可用的网卡")
		return
	}
	label := "启用"
	if !on {
		label = "禁用"
	}
	a.async("正在"+label+"网卡 ["+adapterName+"]…", func() (string, error) {
		if err := netcfg.SetAdapterEnabled(adapterName, on); err != nil {
			return "", err
		}
		time.Sleep(600 * time.Millisecond)
		list, _ := netcfg.List()
		host, _ := hostsys.Current()
		a.post(func() { a.applyRefreshedData(list, host, false) })
		return label + "网卡成功", nil
	})
}

// refreshData re-reads everything asynchronously when a live window exists
// (or synchronously in headless tests) and reports it in the status bar.
func (a *App) refreshData() {
	if a.main == nil {
		a.refreshAdapters()
		a.refreshHost()
		if ad, ok := a.currentAdapterOK(); ok {
			a.Form.LoadFromAdapter(ad, a.host)
			a.loadAdvancedFromAdapter(ad)
			a.Sub.IP = ad.CurrentIP()
			if a.Sub.IP == "" {
				a.Sub.IP = "192.168.1.10"
			}
			a.recalcSubnet()
		}
		a.setStatus(fmt.Sprintf("✓ 已刷新：%d 个网卡", len(a.adapters)))
		return
	}
	a.async("正在刷新网卡与系统信息…", func() (string, error) {
		list, err := netcfg.List()
		if err != nil {
			return "", err
		}
		host, hostErr := hostsys.Current()
		a.post(func() {
			if hostErr != nil {
				a.hostErr = hostErr.Error()
			}
			a.applyRefreshedData(list, host, true)
			a.RefreshTrayMenu()
		})
		return fmt.Sprintf("已刷新：%d 个网卡", len(list)), nil
	})
}

// renameComputer applies the computer name field.
func (a *App) renameComputer() {
	name := strings.TrimSpace(a.Form.Computer)
	if err := hostsys.ValidateName(name); err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	a.async("正在修改计算机名为 "+name+"…", func() (string, error) {
		if _, err := hostsys.Rename(name); err != nil {
			return "", err
		}
		return "计算机名已改为 " + name + "，重启后生效", nil
	})
}

// joinWorkgroup applies the workgroup field.
func (a *App) joinWorkgroup() {
	name := strings.TrimSpace(a.Form.Workgroup)
	if err := hostsys.ValidateWorkgroup(name); err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	a.async("正在修改工作组为 "+name+"…", func() (string, error) {
		if _, err := hostsys.JoinWorkgroup(name); err != nil {
			return "", err
		}
		return "工作组已改为 " + name + "，重启后生效", nil
	})
}

// restartComputer triggers a system restart after confirmation.
func (a *App) restartComputer() {
	a.async("正在调度系统重启（5 秒后）…", func() (string, error) {
		if err := hostsys.Restart(); err != nil {
			return "", err
		}
		return "系统将在 5 秒后重启（可运行 shutdown /a 取消）", nil
	})
}

// setMAC writes the MAC field to the adapter.
func (a *App) setMAC() {
	ad := a.currentAdapter()
	if ad.Name == "" {
		a.setStatus("✗ 没有可用的网卡")
		return
	}
	raw := strings.TrimSpace(a.Form.MAC)
	norm, err := macaddr.Normalize(raw)
	if err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	formatted := macaddr.Format(norm)
	a.Form.MAC = formatted
	a.async("正在修改 ["+ad.Name+"] 的 MAC 地址…", func() (string, error) {
		if err := macaddr.Set(ad, norm); err != nil {
			return "", err
		}
		time.Sleep(500 * time.Millisecond)
		list, _ := netcfg.List()
		host, _ := hostsys.Current()
		a.post(func() { a.applyRefreshedData(list, host, false) })
		return "MAC 已改为 " + formatted + "（网卡已重启）", nil
	})
}

// restoreMAC removes a MAC set earlier.
func (a *App) restoreMAC() {
	ad := a.currentAdapter()
	if ad.Name == "" {
		a.setStatus("✗ 没有可用的网卡")
		return
	}
	a.async("正在还原 ["+ad.Name+"] 的 MAC 地址…", func() (string, error) {
		if err := macaddr.Restore(ad); err != nil {
			return "", err
		}
		time.Sleep(500 * time.Millisecond)
		list, _ := netcfg.List()
		host, _ := hostsys.Current()
		a.post(func() { a.applyRefreshedData(list, host, false) })
		return "MAC 已还原为网卡原始地址（网卡已重启）", nil
	})
}

// saveProfile stores the main form as a profile, replacing one with the same name.
func (a *App) saveProfile(name, location string) error {
	p := a.buildFullProfile(strings.TrimSpace(name), strings.TrimSpace(location))
	return a.saveProfileModel(p, "", true)
}

// saveProfileModel validates and persists p into a.profiles and ip.dat.
// When asNew is false and oldName is non-empty, it updates the entry named oldName
// (handling in-place renames cleanly).
func (a *App) saveProfileModel(p model.Profile, oldName string, asNew bool) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Location = strings.TrimSpace(p.Location)
	if err := p.Validate(); err != nil {
		return err
	}
	p.UpdatedAt = time.Now()
	target := p.Name
	if !asNew && oldName != "" {
		target = oldName
	}
	replaced := false
	for i := range a.profiles {
		if a.profiles[i].Name == target {
			p.Usage = a.profiles[i].Usage
			a.profiles[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		// Also check if another profile already has p.Name to avoid duplicates.
		for i := range a.profiles {
			if a.profiles[i].Name == p.Name {
				p.Usage = a.profiles[i].Usage
				a.profiles[i] = p
				replaced = true
				break
			}
		}
	}
	if !replaced {
		a.profiles = append(a.profiles, p)
	}
	model.SortProfiles(a.profiles)
	if err := a.store.Save(a.profiles); err != nil {
		return err
	}
	a.RefreshTrayMenu()
	return nil
}

// deleteProfile removes a profile by name.
func (a *App) deleteProfile(name string) error {
	var out []model.Profile
	for _, p := range a.profiles {
		if p.Name != name {
			out = append(out, p)
		}
	}
	a.profiles = out
	if err := a.store.Save(a.profiles); err != nil {
		return err
	}
	a.RefreshTrayMenu()
	return nil
}

// applyProfile applies a saved profile and counts the use.
func (a *App) applyProfile(p model.Profile) {
	if a.adapter == "" {
		a.setStatus("✗ 没有可用的网卡")
		return
	}
	a.captureSnapshot(a.adapter)
	p = p.Clone()
	req := netcfg.Request{Adapter: a.adapter, DHCP: p.DHCP}
	if !p.DHCP {
		req.IPv4 = p.IPv4
		req.Masks = p.Masks
		req.Gateways = p.Gateways
	}
	req.DNS = p.DNS
	adapterName := a.adapter
	site := p.Location
	sysName := p.Computer
	a.async("正在应用方案 ["+p.Name+"]…", func() (string, error) {
		if _, err := netcfg.Apply(req); err != nil {
			return "", err
		}
		_ = a.store.AppendLog(store.LogFormat(time.Now(), site, sysName, adapterName, p))
		list, _ := netcfg.List()
		host, _ := hostsys.Current()
		a.post(func() {
			for i := range a.profiles {
				if a.profiles[i].Name == p.Name {
					a.profiles[i].Usage++
				}
			}
			model.SortProfiles(a.profiles)
			_ = a.store.Save(a.profiles)
			a.Form.LoadFrom(p)
			a.loadAdvancedFromForm()
			a.applyRefreshedData(list, host, false)
			a.RefreshTrayMenu()
		})
		return "已应用方案 " + p.Name, nil
	})
}

// currentSummary builds a clean multi-line text summary of the current adapter
// for one-click clipboard copy.
func (a *App) currentSummary() string {
	ad := a.currentAdapter()
	var b strings.Builder
	fmt.Fprintf(&b, "网卡：%s (%s)\n", emptyDash(ad.Name), ad.Status())
	fmt.Fprintf(&b, "MAC：%s\n", emptyDash(macaddr.Format(ad.MAC)))
	if ad.DHCP {
		b.WriteString("方式：DHCP 自动获取\n")
	} else {
		b.WriteString("方式：静态配置\n")
	}
	fmt.Fprintf(&b, "IPv4：%s\n", emptyDash(strings.Join(ad.IPv4, ", ")))
	fmt.Fprintf(&b, "掩码：%s\n", emptyDash(strings.Join(ad.Masks, ", ")))
	fmt.Fprintf(&b, "网关：%s\n", emptyDash(strings.Join(ad.Gateways, ", ")))
	fmt.Fprintf(&b, "DNS：%s\n", emptyDash(strings.Join(ad.DNS, ", ")))
	fmt.Fprintf(&b, "计算机：%s / %s", emptyDash(a.host.Name), emptyDash(a.host.Workgroup))
	return b.String()
}

// recalcSubnet refreshes the subnet calculator's result.
// Also supports CIDR input like "192.168.1.10/24" directly in the IP field.
func (a *App) recalcSubnet() {
	ipStr := strings.TrimSpace(a.Sub.IP)
	if ip, p, ok := subnet.ParseCIDR(ipStr); ok {
		a.Sub.IP = ip
		a.Sub.Prefix = float64(p)
		a.Sub.PrefixText = strconv.Itoa(p)
		ipStr = ip
	}
	prefix := int(a.Sub.Prefix + 0.5)
	if prefix < 0 {
		prefix = 0
	}
	if prefix > 32 {
		prefix = 32
	}
	a.Sub.Prefix = float64(prefix)
	a.Sub.PrefixText = strconv.Itoa(prefix)
	info, err := subnet.ComputePrefix(ipStr, prefix)
	if err != nil {
		a.Sub.Err = err.Error()
		a.Sub.Info = subnet.Info{}
		return
	}
	a.Sub.Err = ""
	a.Sub.Info = info
}

// startPing probes the /24 network of the ping screen with throttled UI batches
// and supports immediate cancellation via stopPing.
func (a *App) startPing() {
	if a.Ping.Running {
		return
	}
	rawPrefix := strings.TrimSpace(a.Ping.Prefix)
	if rawPrefix == "" {
		ip := a.currentAdapter().CurrentIP()
		if i := strings.LastIndexByte(ip, '.'); i > 0 {
			rawPrefix = ip[:i]
			a.Ping.Prefix = rawPrefix
		}
	}
	prefix, err := pingfan.ValidatePrefix(rawPrefix)
	if err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	a.Ping.Prefix = prefix

	workers := a.Ping.Workers
	if n, err := strconv.Atoi(strings.TrimSpace(a.Ping.WorkersText)); err == nil && n >= 1 && n <= 128 {
		workers = n
		a.Ping.Workers = n
	}
	timeoutMs := a.Ping.Timeout
	if n, err := strconv.Atoi(strings.TrimSpace(a.Ping.TimeoutText)); err == nil && n >= 100 && n <= 5000 {
		timeoutMs = float64(n)
		a.Ping.Timeout = timeoutMs
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.Ping.cancel = cancel
	a.Ping.Running = true
	a.Ping.Done = 0
	a.Ping.Online = 0
	a.Ping.State = [256]int{}
	a.Ping.RTT = [256]int{}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	a.setStatus("正在探测 " + prefix + ".1-254 …")

	go func() {
		defer cancel()
		var (
			batchMu sync.Mutex
			pending []pingfan.Result
			done    int
			online  int
		)
		flush := func(final bool) {
			batchMu.Lock()
			items := pending
			pending = nil
			curDone := done
			curOnline := online
			batchMu.Unlock()
			if len(items) == 0 && !final {
				return
			}
			a.post(func() {
				for _, r := range items {
					n := r.Host
					if n <= 0 {
						if i := strings.LastIndexByte(r.IP, '.'); i > 0 {
							fmt.Sscanf(r.IP[i+1:], "%d", &n)
						}
					}
					if n >= 1 && n <= 254 {
						if r.Online {
							a.Ping.State[n] = 1
							a.Ping.RTT[n] = int(r.RTT / time.Millisecond)
						} else {
							a.Ping.State[n] = 2
						}
					}
				}
				a.Ping.Done = curDone
				a.Ping.Online = curOnline
				if final {
					a.Ping.Running = false
					a.Ping.cancel = nil
					if ctx.Err() != nil {
						a.setStatus(fmt.Sprintf("已停止探测（已完成 %d/254，在线 %d 台）", curDone, curOnline))
					} else {
						a.setStatus(fmt.Sprintf("✓ 群 Ping 完成：%s.1-254 在线 %d 台", prefix, curOnline))
					}
				}
			})
		}

		ticker := time.NewTicker(60 * time.Millisecond)
		stopTicker := make(chan struct{})
		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					flush(false)
				case <-stopTicker:
					return
				}
			}
		}()

		pingfan.FanContext(ctx, prefix, workers, timeout, func(r pingfan.Result) {
			batchMu.Lock()
			done++
			if r.Online {
				online++
			}
			pending = append(pending, r)
			batchMu.Unlock()
		})
		close(stopTicker)
		flush(true)
	}()
}

// stopPing cancels an in-progress C-class ping sweep.
func (a *App) stopPing() {
	if a.Ping.cancel != nil {
		a.Ping.cancel()
	}
}

// onlineIPs returns all IPs currently marked online in the ping grid.
func (a *App) onlineIPs() []string {
	prefix := strings.TrimSuffix(strings.TrimSpace(a.Ping.Prefix), ".")
	var out []string
	for n := 1; n <= 254; n++ {
		if a.Ping.State[n] == 1 {
			out = append(out, fmt.Sprintf("%s.%d", prefix, n))
		}
	}
	return out
}

// computeRoute refreshes the route preview.
func (a *App) computeRoute() {
	ad := a.currentAdapter()
	if ad.Name == "" {
		a.Route.Err = "没有可用的网卡"
		a.Route.Preview = nil
		return
	}
	a.Route.Adapter = ad.Name
	a.Route.Err = ""
	direct := routing.IsDirect(ad.CurrentIP(), firstMask(ad.Masks), a.Route.Net, a.Route.Mask)
	plan := routing.Plan{
		Adapter:     ad.Name,
		IfIndex:     adapterIndex(ad),
		Net:         a.Route.Net,
		Mask:        a.Route.Mask,
		Gateway:     a.Route.Gateway,
		Direct:      direct,
		KeepDefault: a.Route.KeepDefault,
		Persist:     a.Route.Persist,
	}
	cmds, err := plan.Commands()
	if err != nil {
		a.Route.Err = err.Error()
		a.Route.Preview = nil
		return
	}
	a.Route.Preview = cmds
}

func firstMask(masks []string) string {
	if len(masks) == 0 {
		return ""
	}
	return masks[0]
}

// adapterIndex returns the adapter's IPv4 interface index, asking netsh when
// the adapter list did not report one.
func adapterIndex(ad netcfg.Adapter) int {
	if ad.Index > 0 {
		return ad.Index
	}
	return routing.InterfaceIndex(ad.Name)
}

// loadRouteTable loads `route print -4` asynchronously and parses structured entries.
func (a *App) loadRouteTable() {
	a.async("正在读取 IPv4 路由表…", func() (string, error) {
		table, err := routing.Table()
		if err != nil {
			return "", err
		}
		entries := routing.ParseTable(table)
		a.post(func() {
			a.Route.Output = table
			a.Route.Entries = entries
			a.Route.Tab = 1
		})
		return fmt.Sprintf("已读取 IPv4 路由表（共 %d 条路由）", len(entries)), nil
	})
}

// applyRoute runs the split route plan.
func (a *App) applyRoute() {
	ad := a.currentAdapter()
	if ad.Name == "" {
		a.setStatus("✗ 没有可用的网卡")
		return
	}
	plan := routing.Plan{
		Adapter:     ad.Name,
		IfIndex:     adapterIndex(ad),
		Net:         a.Route.Net,
		Mask:        a.Route.Mask,
		Gateway:     a.Route.Gateway,
		Direct:      routing.IsDirect(ad.CurrentIP(), firstMask(ad.Masks), a.Route.Net, a.Route.Mask),
		KeepDefault: a.Route.KeepDefault,
		Persist:     a.Route.Persist,
	}
	if err := plan.Validate(); err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	site := a.profileLocation()
	sysName := a.Form.Computer
	a.async("正在配置内网分流路由…", func() (string, error) {
		ran, output, err := routing.Apply(plan)
		note := ""
		if plan.Direct {
			note = "目标网段为网卡直连，未添加路由"
		}
		if err != nil {
			return output, err
		}
		table, _ := routing.Table()
		entries := routing.ParseTable(table)
		_ = a.store.AppendLog(store.LogRoute(time.Now(), site, sysName, plan.Adapter, plan.Net, plan.Mask, plan.Gateway, note))
		summary := "已执行：" + strings.Join(ran, "；")
		a.post(func() {
			a.Route.Output = output + "\n" + table
			a.Route.Entries = entries
			a.Route.Done = true
		})
		return summary, nil
	})
}

// advancedApply applies the advanced page: only the current tab when pageOnly
// is set, otherwise the whole form with the metric.
func (a *App) advancedApply(pageOnly bool) {
	a.formFromAdvanced()
	ad := a.currentAdapter()
	if ad.Name == "" {
		a.setStatus("✗ 没有可用的网卡")
		return
	}
	if pageOnly {
		a.applyCurrentPage(ad)
		return
	}
	if a.Adv.MetricAuto {
		a.applyNow()
		return
	}
	metric, err := strconv.Atoi(strings.TrimSpace(a.Adv.MetricCustom))
	if err != nil || metric < 1 || metric > 9999 {
		a.setStatus("✗ 自定义跃点数需要 1..9999 的数字")
		return
	}
	// Snapshot immutable values on the UI thread to prevent data races.
	req, err := a.request()
	if err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	profile := a.buildFullProfile(a.profileName(), a.profileLocation())
	if err := profile.Validate(); err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	adapterName := ad.Name
	a.captureSnapshot(adapterName)
	a.async(fmt.Sprintf("正在设置 [%s] 跃点数为 %d 并应用配置…", adapterName, metric), func() (string, error) {
		if err := netcfg.SetAdapterMetric(adapterName, metric); err != nil {
			return "", err
		}
		if _, err := netcfg.Apply(req); err != nil {
			return "", err
		}
		_ = a.store.AppendLog(store.LogFormat(time.Now(), profile.Location, profile.Computer, adapterName, profile))
		list, _ := netcfg.List()
		host, _ := hostsys.Current()
		a.post(func() { a.applyRefreshedData(list, host, false) })
		return "跃点数与配置已应用", nil
	})
}

// applyCurrentPage applies only what the open tab edits, leaving the adapter's
// other settings alone.
func (a *App) applyCurrentPage(ad netcfg.Adapter) {
	switch a.Adv.Tab {
	case 0:
		if len(a.Adv.IPs) == 0 {
			a.setStatus("✗ IP 地址页没有条目")
			return
		}
		a.captureSnapshot(ad.Name)
		req := netcfg.Request{Adapter: ad.Name}
		for _, row := range a.Adv.IPs {
			req.IPv4 = append(req.IPv4, strings.TrimSpace(row.Value))
			req.Masks = append(req.Masks, strings.TrimSpace(row.Mask))
		}
		for _, row := range a.Adv.GWs {
			if v := strings.TrimSpace(row.Value); v != "" {
				metric, _ := strconv.Atoi(strings.TrimSpace(row.Metric))
				req.Gateways = append(req.Gateways, model.Gateway{Addr: v, Metric: metric})
			}
		}
		a.async("正在仅应用 IP 地址页…", func() (string, error) {
			if _, err := netcfg.ApplyAddresses(req); err != nil {
				return "", err
			}
			list, _ := netcfg.List()
			host, _ := hostsys.Current()
			a.post(func() { a.applyRefreshedData(list, host, false) })
			return "已应用 IP 地址页（DNS 未改动）", nil
		})
	case 1:
		if len(a.Adv.GWs) == 0 {
			a.setStatus("✗ 网关地址页没有条目")
			return
		}
		var gateways []model.Gateway
		for _, row := range a.Adv.GWs {
			v := strings.TrimSpace(row.Value)
			if v == "" {
				continue
			}
			var metric int
			if ms := strings.TrimSpace(row.Metric); ms != "" {
				m, err := strconv.Atoi(ms)
				if err != nil || m < 0 || m > 9999 {
					a.setStatus(fmt.Sprintf("✗ 网关 %s 跃点数无效：%s", v, ms))
					return
				}
				metric = m
			}
			gateways = append(gateways, model.Gateway{Addr: v, Metric: metric})
		}
		if len(gateways) == 0 {
			a.setStatus("✗ 网关地址页没有有效条目")
			return
		}
		a.captureSnapshot(ad.Name)
		a.async("正在仅应用网关地址页…", func() (string, error) {
			if _, err := netcfg.ApplyGateways(ad.Name, gateways); err != nil {
				return "", err
			}
			list, _ := netcfg.List()
			host, _ := hostsys.Current()
			a.post(func() { a.applyRefreshedData(list, host, false) })
			return fmt.Sprintf("已应用 %d 个网关", len(gateways)), nil
		})
	case 2:
		var servers []string
		for _, row := range a.Adv.DNS {
			if v := strings.TrimSpace(row.Value); v != "" {
				servers = append(servers, v)
			}
		}
		a.captureSnapshot(ad.Name)
		a.async("正在仅应用 DNS 服务器页…", func() (string, error) {
			if _, err := netcfg.ApplyDNS(ad.Name, servers); err != nil {
				return "", err
			}
			list, _ := netcfg.List()
			host, _ := hostsys.Current()
			a.post(func() { a.applyRefreshedData(list, host, false) })
			if len(servers) == 0 {
				return "已恢复自动获取 DNS", nil
			}
			return fmt.Sprintf("已应用 %d 个 DNS 服务器", len(servers)), nil
		})
	default:
		a.setStatus("✗ 「其他功能」页没有可单独应用的列表")
	}
}

// reloadLog reads the tail of network_config.log for the LogView window.
func (a *App) reloadLog() {
	text, err := a.store.ReadRecentLog(300)
	if err != nil {
		a.LogText = "读取日志失败：" + err.Error()
		return
	}
	a.LogText = text
}

// releaseIP, renewIP and flushDNS are the advanced page's ipconfig actions.
func (a *App) releaseIP() {
	ad := a.currentAdapter()
	if ad.Name == "" {
		a.setStatus("✗ 没有可用的网卡")
		return
	}
	a.async("正在释放 IP 地址…", func() (string, error) {
		out, err := netcfg.Release(ad.Name)
		list, _ := netcfg.List()
		host, _ := hostsys.Current()
		a.post(func() { a.applyRefreshedData(list, host, false) })
		return strings.TrimSpace(out), err
	})
}

func (a *App) renewIP() {
	ad := a.currentAdapter()
	if ad.Name == "" {
		a.setStatus("✗ 没有可用的网卡")
		return
	}
	a.async("正在更新 IP 地址…", func() (string, error) {
		out, err := netcfg.Renew(ad.Name)
		list, _ := netcfg.List()
		host, _ := hostsys.Current()
		a.post(func() { a.applyRefreshedData(list, host, false) })
		return strings.TrimSpace(out), err
	})
}

func (a *App) flushDNS() {
	a.async("正在清除 DNS 解析程序缓存…", func() (string, error) {
		out, err := netcfg.FlushDNS()
		return strings.TrimSpace(out), err
	})
}

// openNetworkConnections launches ncpa.cpl without blocking the UI thread.
func (a *App) openNetworkConnections() {
	if err := netcfg.OpenNetworkConnections(); err != nil {
		a.setStatus("✗ 打开网络连接失败：" + err.Error())
		return
	}
	a.setStatus("已打开 Windows 网络连接")
}
