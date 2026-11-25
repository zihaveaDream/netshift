package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"netshift/internal/model"
	"netshift/internal/netcfg"
	"netshift/internal/routing"
	"netshift/internal/sysnet"
)

// newTestApp returns an app whose files live in a temporary directory.
func newTestApp(t *testing.T) *App {
	t.Helper()
	return New(t.TempDir(), "test")
}

func TestMainViewBuilds(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.MainView, 820, 460)
	for _, want := range []string{
		"选择硬件", "网络适配器:", "启用网卡", "禁用网卡", "刷新数据",
		"计算机信息", "计算机名:", "工作组:", "修改计算机名", "修改工作组",
		"网卡 MAC 地址管理", "MAC 地址:", "随机生成MAC地址", "还原MAC地址",
		"Internet 协议版本 4 (TCP/IPv4) 属性", "自动获取地址", "使用以下地址",
		"IP 地址:", "子网掩码:", "默认网关:", "首选 DNS:", "备用 DNS:",
		"应用配置", "恢复自动获取", "导入配置", "保存当前配置", "高级选项", "工具箱",
	} {
		if !tt.HasText(want) {
			t.Errorf("main window is missing %q", want)
		}
	}
}

func TestRandomMACButton(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.MainView, 820, 460)
	if err := tt.Click("随机生成MAC地址"); err != nil {
		t.Fatal(err)
	}
	if app.Form.MAC == "" {
		t.Fatal("the MAC field stayed empty")
	}
	if len(strings.ReplaceAll(app.Form.MAC, "-", "")) != 12 {
		t.Errorf("generated MAC %q is not 12 hex digits", app.Form.MAC)
	}
}

func TestRandomComputerNameButton(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.MainView, 820, 460)
	if err := tt.Click("随机生成计算机名"); err != nil {
		t.Fatal(err)
	}
	if app.Form.Computer == "" {
		t.Error("the computer name stayed empty")
	}
}

func TestModeRadioSwitches(t *testing.T) {
	app := newTestApp(t)
	app.Form.Mode = 1
	tt := ui.NewTester(app.MainView, 820, 460)
	if err := tt.Click("自动获取地址"); err != nil {
		t.Fatal(err)
	}
	if app.Form.Mode != 0 {
		t.Errorf("mode = %d, want 0", app.Form.Mode)
	}
	if err := tt.Click("使用以下地址"); err != nil {
		t.Fatal(err)
	}
	if app.Form.Mode != 1 {
		t.Errorf("mode = %d, want 1", app.Form.Mode)
	}
}

func TestSubnetViewComputes(t *testing.T) {
	app := newTestApp(t)
	app.Sub.IP = "192.168.1.10"
	app.Sub.Prefix = 24
	app.recalcSubnet()
	tt := ui.NewTester(app.SubnetView, 460, 520)
	if !tt.HasText("192.168.1.0") {
		t.Errorf("network address missing, texts: %v", tt.Texts())
	}
	if !tt.HasText("192.168.1.255") {
		t.Errorf("broadcast address missing, texts: %v", tt.Texts())
	}
	if !tt.HasText("254") {
		t.Errorf("host count missing, texts: %v", tt.Texts())
	}
	if !tt.HasText("255.255.255.0") {
		t.Errorf("mask missing, texts: %v", tt.Texts())
	}
}

func TestAdvancedViewTabs(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.AdvancedView, 620, 480)
	for _, want := range []string{"IP 地址", "网关地址", "DNS 服务器", "其他功能", "应用", "返回"} {
		if !tt.HasText(want) {
			t.Errorf("advanced view is missing %q", want)
		}
	}
	app.Adv.Tab = 1
	tt.Frame()
	if !tt.HasText("第一个网关写入主界面") {
		t.Errorf("gateway page not shown, texts: %v", tt.Texts())
	}
	app.Adv.Tab = 3
	tt.Frame()
	for _, want := range []string{"修改适配器跃点数", "自动", "自定义", "释放适配器的 IP 地址", "更新适配器的 IP 地址", "清除 DNS 解析程序缓存"} {
		if !tt.HasText(want) {
			t.Errorf("other page is missing %q", want)
		}
	}
}

func TestAdvancedAddsAddressRows(t *testing.T) {
	app := newTestApp(t)
	app.Adv.Tab = 0
	tt := ui.NewTester(app.AdvancedView, 620, 480)
	app.Adv.Draft.Value = "10.1.2.3"
	app.Adv.Draft.Mask = "255.255.0.0"
	tt.Frame()
	if err := tt.Click("添加"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range app.Adv.IPs {
		if row.Value == "10.1.2.3" && row.Mask == "255.255.0.0" {
			found = true
		}
	}
	if !found {
		t.Errorf("the row was not added: %+v", app.Adv.IPs)
	}
}

func TestPresetsSavesProfile(t *testing.T) {
	app := newTestApp(t)
	app.Form.Mode = 1
	app.Form.IP = "192.168.5.10"
	app.Form.Mask = "255.255.255.0"
	app.Form.Gateway = "192.168.5.1"
	app.Form.DNS1 = "192.168.5.1"
	app.Pre.Name = "测试方案"
	app.Pre.Location = "测试地点"
	app.Pre.Edit = app.Form
	tt := ui.NewTester(app.PresetsView, 720, 520)
	if err := tt.Click("另存为新方案"); err != nil {
		t.Fatal(err)
	}
	if len(app.profiles) != 1 {
		t.Fatalf("saved %d profiles, want 1", len(app.profiles))
	}
	if app.profiles[0].Name != "测试方案" || app.profiles[0].PrimaryIP() != "192.168.5.10" {
		t.Errorf("profile = %+v", app.profiles[0])
	}
	// The file must round-trip through the store.
	reloaded, err := app.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded) != 1 || reloaded[0].Location != "测试地点" {
		t.Errorf("reloaded = %+v", reloaded)
	}
}

func TestPresetsDeleteProfile(t *testing.T) {
	app := newTestApp(t)
	if err := app.saveProfile("待删除", "某地"); err != nil {
		t.Fatal(err)
	}
	app.Pre.Name = "待删除"
	tt := ui.NewTester(app.PresetsView, 720, 520)
	if err := tt.Click("删除当前方案"); err != nil {
		t.Fatal(err)
	}
	if len(app.profiles) != 0 {
		t.Errorf("profiles left: %+v", app.profiles)
	}
}

func TestInheritFromLocation(t *testing.T) {
	app := newTestApp(t)
	older := model.Profile{Name: "办公室-旧", Location: "北京", IPv4: []string{"192.168.1.20"}, Masks: []string{"255.255.255.0"},
		DNS: []string{"192.168.1.1"}, Usage: 1}
	newer := model.Profile{Name: "办公室-新", Location: "北京", IPv4: []string{"192.168.1.30"}, Masks: []string{"255.255.0.0"},
		Gateways: []model.Gateway{{Addr: "192.168.0.1"}}, DNS: []string{"192.168.0.1", "8.8.8.8"}, Usage: 9}
	other := model.Profile{Name: "别处", Location: "上海", IPv4: []string{"10.0.0.5"}, Masks: []string{"255.0.0.0"}, Usage: 99}
	app.profiles = []model.Profile{older, newer, other}

	app.Pre.Location = "北京"
	app.Pre.Name = "办公室-新2"
	app.inheritFromLocation()

	if app.Pre.Edit.Gateway != "192.168.0.1" {
		t.Errorf("gateway = %q, want the most used profile's", app.Pre.Edit.Gateway)
	}
	if app.Pre.Edit.Mask != "255.255.0.0" || app.Pre.Edit.DNS1 != "192.168.0.1" || app.Pre.Edit.DNS2 != "8.8.8.8" {
		t.Errorf("inherited fields = %+v", app.Pre.Edit)
	}

	app.Pre.Location = "广州"
	app.inheritFromLocation()
	if app.Pre.Edit.Gateway != "192.168.0.1" {
		t.Error("a location with no profiles should leave the editor alone")
	}
}

func TestToolboxView(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.ToolboxView, 480, 560)
	for _, want := range []string{"C网群Ping器", "网络连通性诊断", "子网掩码计算器", "内网分流路由", "代理与 Hosts 管理", "操作日志", "打开网络连接", "关于"} {
		if !tt.HasText(want) {
			t.Errorf("toolbox is missing %q", want)
		}
	}
}

func TestSnapshotAndPingSelectToMainForm(t *testing.T) {
	app := newTestApp(t)
	app.adapters = []netcfg.Adapter{{
		Name:     "以太网",
		IPv4:     []string{"192.168.10.20"},
		Masks:    []string{"255.255.255.0"},
		Gateways: []string{"192.168.10.1"},
		Up:       true,
	}}
	app.adapter = "以太网"
	app.captureSnapshot("以太网")
	snap, ok := app.currentSnapshot()
	if !ok || snap.Summary != "192.168.10.20" {
		t.Fatalf("snapshot = %+v, ok=%v", snap, ok)
	}

	app.Ping.Prefix = "10.0.5"
	app.Ping.SelectedHost = 88
	app.Form.Gateway = ""
	app.useSelectedPingIP()
	if app.Form.IP != "10.0.5.88" || app.Form.Mode != 1 {
		t.Errorf("useSelectedPingIP failed: %+v", app.Form)
	}
}

func TestDiagAndSysNetViewsBuild(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.DiagView, 680, 560)
	for _, want := range []string{"基础连通性诊断", "一键开始诊断", "TCP 端口连通探测", "探测端口"} {
		if !tt.HasText(want) {
			t.Errorf("diag view missing %q", want)
		}
	}

	app.SysNet.Tab = 0
	tt2 := ui.NewTester(app.SysNetView, 760, 580)
	for _, want := range []string{"系统代理开关", "Hosts 快捷管理", "一键关闭代理", "保存并立即生效"} {
		if !tt2.HasText(want) {
			t.Errorf("sysnet proxy tab missing %q", want)
		}
	}

	app.SysNet.Tab = 1
	app.SysNet.Hosts = sysnet.ParseHosts("hosts", "127.0.0.1 localhost\n# 10.0.0.5 api.local # test\n")
	tt2.Frame()
	if !tt2.HasText("保存并刷新 DNS") {
		t.Errorf("sysnet hosts tab missing save button: %v", tt2.Texts())
	}
}

func TestRouteViewStructuredFilter(t *testing.T) {
	app := newTestApp(t)
	app.Route.Entries = []routing.RouteEntry{
		{Dest: "0.0.0.0", Mask: "0.0.0.0", Gateway: "192.168.1.1", Metric: "25"},
		{Dest: "10.0.0.0", Mask: "255.0.0.0", Gateway: "192.168.1.1", Metric: "1", Persistent: true},
	}
	app.Route.OnlyPersistent = true
	got := app.filteredRoutes()
	if len(got) != 1 || got[0].Dest != "10.0.0.0" {
		t.Errorf("filteredRoutes = %+v", got)
	}
}

func TestPresetsSaveEditorUsesEditorNotMainForm(t *testing.T) {
	app := newTestApp(t)
	app.Form.Mode = 1
	app.Form.IP = "10.0.0.1"
	app.Form.Mask = "255.0.0.0"

	app.Pre.Name = "独立方案"
	app.Pre.Location = "深圳"
	app.Pre.Edit = Form{
		Mode:    1,
		IP:      "192.168.88.10",
		Mask:    "255.255.255.0",
		Gateway: "192.168.88.1",
		DNS1:    "223.5.5.5",
	}
	app.saveEditor(false)
	if len(app.profiles) != 1 {
		t.Fatalf("saved %d profiles, want 1", len(app.profiles))
	}
	got := app.profiles[0]
	if got.PrimaryIP() != "192.168.88.10" || got.PrimaryGateway() != "192.168.88.1" || got.PrimaryDNS() != "223.5.5.5" {
		t.Errorf("saveEditor saved wrong fields (main form leaked?): %+v", got)
	}
}

func TestRequestPreservesMultiIPAndMainFormDNS(t *testing.T) {
	app := newTestApp(t)
	app.adapter = "以太网"
	app.Form.Mode = 1
	app.Form.IP = "192.168.1.10"
	app.Form.Mask = "255.255.255.0"
	app.Form.Gateway = "192.168.1.1"
	app.Form.DNS1 = "223.5.5.5"
	app.Form.DNS2 = "114.114.114.114"
	app.Adv.IPs = []AddressRow{
		{Value: "192.168.1.10", Mask: "255.255.255.0"},
		{Value: "10.20.30.40", Mask: "255.255.0.0"},
	}
	app.Adv.DNS = []AddressRow{{Value: "1.1.1.1"}} // stale DNS in Adv should be overridden by Form

	req, err := app.request()
	if err != nil {
		t.Fatal(err)
	}
	if len(req.IPv4) != 2 || req.IPv4[1] != "10.20.30.40" {
		t.Errorf("multi-IP lost: %+v", req.IPv4)
	}
	if len(req.DNS) != 2 || req.DNS[0] != "223.5.5.5" || req.DNS[1] != "114.114.114.114" {
		t.Errorf("main form DNS not synced: %+v", req.DNS)
	}
}

func TestDHCPModeIgnoresDisabledStaticDNS(t *testing.T) {
	app := newTestApp(t)
	app.adapter = "以太网"
	app.Form.Mode = 0 // 自动获取地址（输入框全部禁用）
	app.Form.IP = "192.168.1.10"
	app.Form.Mask = "255.255.255.0"
	app.Form.DNS1 = "invalid-leftover-dns"
	app.Adv.DNS = []AddressRow{{Value: "invalid-leftover-dns"}}

	req, err := app.request()
	if err != nil {
		t.Fatalf("request() failed in DHCP mode: %v", err)
	}
	if !req.DHCP || len(req.IPv4) != 0 || len(req.DNS) != 0 {
		t.Errorf("unexpected DHCP request: %+v", req)
	}
	profile := app.buildFullProfile("以太网", "")
	if err := profile.Validate(); err != nil {
		t.Errorf("DHCP profile validation blocked by stale disabled DNS: %v", err)
	}
}

func TestLogViewBuilds(t *testing.T) {
	app := newTestApp(t)
	app.LogText = "时间：2026-11-25 12:00:00\nIP地址：192.168.1.10"
	tt := ui.NewTester(app.LogView, 700, 560)
	if !tt.HasText("操作日志") || !tt.HasText("192.168.1.10") {
		t.Errorf("log view missing expected content: %v", tt.Texts())
	}
}

func TestPingViewBuilds(t *testing.T) {
	app := newTestApp(t)
	app.Ping.Prefix = "192.168.1"
	tt := ui.NewTester(app.PingView, 740, 740)
	for _, want := range []string{"网段:", "开始探测", "按当前网卡填入", "1", "254", "进度"} {
		if !tt.HasText(want) {
			t.Errorf("ping view is missing %q", want)
		}
	}
}

func TestRouteViewPreview(t *testing.T) {
	app := newTestApp(t)
	app.Route.Net = "10.0.0.0"
	app.Route.Mask = "255.0.0.0"
	app.Route.Gateway = "192.168.1.1"
	app.computeRoute()
	tt := ui.NewTester(app.RouteView, 700, 660)
	if !tt.HasText("分流设置") {
		t.Errorf("route view is missing the settings group: %v", tt.Texts())
	}
	if len(app.Route.Preview) == 0 {
		t.Errorf("no preview was computed: %v", app.Route.Err)
	}
}

func TestAboutView(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.AboutView, 420, 260)
	for _, want := range []string{"NetShift", "kn007", "mygo"} {
		if !tt.HasText(want) {
			t.Errorf("about window is missing %q", want)
		}
	}
}
