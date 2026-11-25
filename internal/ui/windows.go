package ui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netshift/internal/assets"
	"netshift/internal/netcfg"
	"netshift/internal/routing"
)

// fitToWorkArea clamps a desired window size (and minimum size) to the current
// display's available work area in DIPs so no dialog ever overflows a scaled
// screen (e.g. 125% / 150% DPI laptops).
func fitToWorkArea(wantW, wantH, minW, minH int) (w, h, outMinW, outMinH int) {
	w, h = wantW, wantH
	workW, workH, _ := netcfg.WorkAreaDIPs()
	if workW > 0 && workH > 0 {
		if maxW := workW - 48; maxW > 320 && w > maxW {
			w = maxW
		}
		if maxH := workH - 48; maxH > 240 && h > maxH {
			h = maxH
		}
	}
	outMinW = minInt(w, minW)
	outMinH = minInt(h, minH)
	return w, h, outMinW, outMinH
}

// openWindow shows a window of the app, creating it the first time, sized to
// fit both its content and the screen's work area.
func (a *App) openWindow(key, title string, wantW, wantH, minW, minH int, view func(*ui.Context)) *mygo.Window {
	if w, ok := a.wins[key]; ok && w != nil && !w.IsDestroyed() {
		w.Show()
		w.Focus()
		w.Invalidate()
		return w
	}
	width, height, mw, mh := fitToWorkArea(wantW, wantH, minW, minH)
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     title,
		Width:     width,
		Height:    height,
		Parent:    a.main,
		MinWidth:  mw,
		MinHeight: mh,
		Content:   ui.View(view),
	})
	_ = w.SetIcon(assets.IconPNG)
	a.wins[key] = w
	w.OnClosed(func() {
		if key == "ping" {
			a.stopPing()
		}
		if a.wins[key] == w {
			delete(a.wins, key)
		}
	})
	return w
}

// closeWindow closes a window of the app by key.
func (a *App) closeWindow(key string) {
	if key == "ping" {
		a.stopPing()
	}
	if w, ok := a.wins[key]; ok && w != nil && !w.IsDestroyed() {
		w.Close()
	}
	delete(a.wins, key)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// IdealAdvancedSize returns the ideal size of the advanced options window
// based on the number of rows currently configured.
func (a *App) IdealAdvancedSize() (int, int) {
	rows := len(a.Adv.IPs)
	if len(a.Adv.GWs) > rows {
		rows = len(a.Adv.GWs)
	}
	if len(a.Adv.DNS) > rows {
		rows = len(a.Adv.DNS)
	}
	h := 480
	if rows > 4 {
		h += (rows - 4) * 34
		if h > 640 {
			h = 640
		}
	}
	return 760, h
}

// IdealPresetsSize returns the ideal size of the presets manager window.
func (a *App) IdealPresetsSize() (int, int) {
	return 960, 640
}

// IdealRouteSize returns the ideal size of the split-routing window, adapting
// width to the current adapter's name length.
func (a *App) IdealRouteSize() (int, int) {
	w := 800
	if ad := a.currentAdapter(); ad.Name != "" {
		need := int(estimateLabelWidthDIP(ad.Name)) + 420
		if need > w {
			w = need
		}
		if w > 980 {
			w = 980
		}
	}
	h := 720
	if a.Route.Output != "" {
		h = 740
	}
	return w, h
}

// openAdvanced shows the advanced options window.
func (a *App) openAdvanced() {
	a.syncFormToAdvanced()
	w, h := a.IdealAdvancedSize()
	a.openWindow("advanced", "高级选项 - NetShift", w, h, 680, 460, a.AdvancedView)
}

// openPresets shows the presets window.
func (a *App) openPresets() {
	w, h := a.IdealPresetsSize()
	a.openWindow("presets", "预设配置方案 - NetShift", w, h, 880, 560, a.PresetsView)
}

// openToolbox shows the toolbox window.
func (a *App) openToolbox() {
	a.openWindow("toolbox", "工具箱 - NetShift", 480, 560, 440, 480, a.ToolboxView)
}

// openDiag shows the network connectivity diagnostic window.
func (a *App) openDiag() {
	if ad := a.currentAdapter(); len(ad.Gateways) > 0 && a.Diag.PortHost == "192.168.1.1" {
		a.Diag.PortHost = ad.Gateways[0]
	}
	a.openWindow("diag", "网络连通性诊断 - NetShift", 680, 600, 580, 480, a.DiagView)
}

// openSysNet shows the Windows system proxy & hosts file manager window.
func (a *App) openSysNet() {
	a.loadSysNet()
	a.openWindow("sysnet", "代理与 Hosts 管理 - NetShift", 760, 580, 660, 480, a.SysNetView)
}

// openSubnet shows the subnet calculator.
func (a *App) openSubnet() {
	a.recalcSubnet()
	a.openWindow("subnet", "子网掩码计算器 - NetShift", 540, 620, 480, 560, a.SubnetView)
}

// openPing shows the C-class ping screen.
func (a *App) openPing() {
	if a.Ping.Prefix == "" {
		ip := a.currentAdapter().CurrentIP()
		if i := lastDot(ip); i > 0 {
			a.Ping.Prefix = ip[:i]
		} else {
			a.Ping.Prefix = "192.168.1"
		}
	}
	a.openWindow("ping", "C网群Ping器 - NetShift", 772, 712, 720, 580, a.PingView)
}

// openRoute shows the split-routing screen.
func (a *App) openRoute() {
	ad := a.currentAdapter()
	if a.Route.Net == "" {
		cands := routeCandidates(ad.CurrentIP())
		if len(cands) > 0 {
			a.Route.Net = cands[0].Net
			a.Route.Mask = cands[0].Mask
			a.Route.NetChoice = cands[0].Label
			a.Route.MaskChoice = maskLabel(cands[0].Mask)
		}
		a.Route.Gateway = routing.GatewayFor(ad.Gateways, a.savedGatewayForLocation(), a.Route.Net)
		a.computeRoute()
	} else {
		if a.Route.MaskChoice == "" {
			a.Route.MaskChoice = maskLabel(a.Route.Mask)
		}
		if a.Route.NetChoice == "" {
			a.Route.NetChoice = "（手动填写）"
		}
	}
	w, h := a.IdealRouteSize()
	a.openWindow("route", "内网分流路由 - NetShift", w, h, 720, 580, a.RouteView)
}

// openLog shows the operation log viewer.
func (a *App) openLog() {
	a.reloadLog()
	a.openWindow("log", "操作日志 - NetShift", 740, 540, 600, 420, a.LogView)
}

// openAbout shows the about window.
func (a *App) openAbout() {
	a.openWindow("about", "关于 NetShift", 500, 320, 440, 290, a.AboutView)
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}
