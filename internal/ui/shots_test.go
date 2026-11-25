package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netshift/internal/model"
	"netshift/internal/netcfg"
	"netshift/internal/netdiag"
	"netshift/internal/routing"
	"netshift/internal/sysnet"
)

// TestCaptureFrames writes a PNG of every window the app draws, for looking at
// the layout outside a GPU. It is skipped unless NETSHIFT_SHOTS names a
// directory:
//
//	$env:NETSHIFT_SHOTS='.tmp\shots'; go test -run TestCaptureFrames ./internal/ui
func TestCaptureFrames(t *testing.T) {
	dir := os.Getenv("NETSHIFT_SHOTS")
	if dir == "" {
		t.Skip("set NETSHIFT_SHOTS to a directory to write the frames there")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newTestApp(t)
	app.version = "5.3.0"
	app.elevated = true
	// Use deterministic synthetic adapters so generated screenshots never leak
	// real machine NIC names, IPs or hardware MAC addresses.
	app.adapters = []netcfg.Adapter{
		{
			Name:          "以太网",
			Description:   "PCIe GbE Controller",
			Index:         12,
			Up:            true,
			DHCP:          false,
			MAC:           "021A2B3C4D5E",
			IfType:        6,
			LinkSpeedMbps: 1000,
			IPv4:          []string{"192.168.1.10"},
			Masks:         []string{"255.255.255.0"},
			Gateways:      []string{"192.168.1.1"},
			DNS:           []string{"223.5.5.5", "114.114.114.114"},
		},
		{
			Name:          "WLAN",
			Description:   "Wi-Fi 6 Wireless Adapter",
			Index:         18,
			Up:            true,
			DHCP:          true,
			MAC:           "021A2B3C4D5F",
			IfType:        71,
			LinkSpeedMbps: 866,
			IPv4:          []string{"192.168.10.25"},
			Masks:         []string{"255.255.255.0"},
			Gateways:      []string{"192.168.10.1"},
			DNS:           []string{"192.168.10.1"},
		},
	}
	app.adapter = app.adapters[0].Name
	app.adapterLabel = app.adapters[0].Label()
	app.profiles = []model.Profile{
		{
			Name:     "北京总部-办公",
			Location: "北京总部",
			IPv4:     []string{"192.168.1.10"},
			Masks:    []string{"255.255.255.0"},
			Gateways: []model.Gateway{{Addr: "192.168.1.1"}},
			DNS:      []string{"223.5.5.5", "114.114.114.114"},
			Usage:    12,
		},
		{
			Name:     "上海机房-核心网",
			Location: "上海机房",
			IPv4:     []string{"10.20.30.40"},
			Masks:    []string{"255.255.252.0"},
			Gateways: []model.Gateway{{Addr: "10.20.28.1"}},
			DNS:      []string{"10.20.0.53"},
			Usage:    7,
		},
		{
			Name:     "客户现场-自动获取",
			Location: "外勤通用",
			DHCP:     true,
			Usage:    5,
		},
	}
	app.Form.Mode = 1
	app.Form.IP = "192.168.1.10"
	app.Form.Mask = "255.255.255.0"
	app.Form.Gateway = "192.168.1.1"
	app.Form.DNS1 = "223.5.5.5"
	app.Form.DNS2 = "114.114.114.114"
	app.Form.Computer = "PC-0001"
	app.Form.Workgroup = "WORKGROUP"
	app.Form.MAC = "02-1A-2B-3C-4D-5E"
	app.Sub.IP = "192.168.1.10"
	app.recalcSubnet()
	app.captureSnapshot(app.adapter)
	app.setStatus("就绪")

	mainW, mainH := app.IdealMainSize()
	advW, advH := app.IdealAdvancedSize()
	preW, preH := app.IdealPresetsSize()
	routeW, routeH := app.IdealRouteSize()
	cases := []struct {
		name string
		w, h int
		view func(*ui.Context)
	}{
		{"main", mainW, mainH, app.MainView},
		{"advanced-ip", advW, advH, app.AdvancedView},
		{"advanced-gw", advW, advH, app.AdvancedView},
		{"advanced-dns", advW, advH, app.AdvancedView},
		{"advanced-other", advW, advH, app.AdvancedView},
		{"presets", preW, preH, app.PresetsView},
		{"toolbox", 480, 560, app.ToolboxView},
		{"diag", 680, 600, app.DiagView},
		{"sysnet-proxy", 760, 580, app.SysNetView},
		{"sysnet-hosts", 760, 580, app.SysNetView},
		{"subnet", 540, 620, app.SubnetView},
		{"ping", 772, 712, app.PingView},
		{"route", routeW, routeH, app.RouteView},
		{"route-table", routeW, routeH, app.RouteView},
		{"log", 740, 540, app.LogView},
		{"about", 500, 320, app.AboutView},
	}
	for _, tc := range cases {
		func() {
			switch tc.name {
			case "advanced-ip":
				app.Adv.Tab = 0
				app.Adv.LastTab = 0
				app.Adv.IPs = []AddressRow{
					{Value: "192.168.1.10", Mask: "255.255.255.0"},
					{Value: "10.20.30.40", Mask: "255.255.252.0"},
				}
				app.Adv.GWs = []AddressRow{{Value: "192.168.1.1", Metric: "1"}}
				app.Adv.DNS = []AddressRow{{Value: "192.168.1.1"}, {Value: "8.8.8.8"}}
			case "advanced-gw":
				app.Adv.Tab = 1
				app.Adv.LastTab = 1
			case "advanced-dns":
				app.Adv.Tab = 2
				app.Adv.LastTab = 2
			case "advanced-other":
				app.Adv.Tab = 3
				app.Adv.LastTab = 3
			case "diag":
				app.Diag.Report = netdiag.Report{
					At: time.Date(2026, 11, 25, 15, 25, 0, 0, time.Local),
					Items: []netdiag.CheckItem{
						{Name: "默认网关", Target: "192.168.1.1", OK: true, Detail: "连通正常（1 ms）"},
						{Name: "DNS 解析", Target: "www.baidu.com", OK: true, Detail: "→ 180.101.50.242（12 ms）"},
						{Name: "外网连通", Target: "223.5.5.5", OK: true, Detail: "ICMP 可达（8 ms）"},
					},
				}
				app.Diag.PortResults = []netdiag.CheckItem{
					{Name: "TCP 端口", Target: "192.168.1.1:80", OK: true, Detail: "端口开放（2 ms）"},
					{Name: "TCP 端口", Target: "192.168.1.1:3389", OK: false, Detail: "连接失败（2001 ms）"},
				}
			case "sysnet-proxy":
				app.SysNet.Tab = 0
				app.SysNet.Proxy = sysnet.ProxyConfig{
					Enabled:  true,
					Server:   "127.0.0.1:7890",
					Override: "localhost;127.*;10.*;192.168.*;<local>",
				}
			case "sysnet-hosts":
				app.SysNet.Tab = 1
				app.SysNet.Hosts = sysnet.ParseHosts(`C:\Windows\System32\drivers\etc\hosts`,
					"127.0.0.1 localhost\n10.20.0.10 portal.corp.local # 内网门户\n# 10.20.0.11 staging.corp.local # 测试环境\n")
			}
			if tc.name == "presets" {
				app.Pre.Name = "北京总部-办公"
				app.Pre.Location = "北京总部"
				app.Pre.Edit = app.Form
			}
			if tc.name == "ping" {
				app.Ping.Prefix = "192.168.1"
				app.Ping.Done = 128
				app.Ping.Online = 4
				app.Ping.SelectedHost = 55
				for _, h := range []int{1, 10, 42, 100} {
					app.Ping.State[h] = 1
					app.Ping.RTT[h] = 2
				}
				for h := 2; h <= 128; h++ {
					if app.Ping.State[h] == 0 {
						app.Ping.State[h] = 2
					}
				}
			}
			if tc.name == "route" || tc.name == "route-table" {
				app.Route.Tab = 0
				if tc.name == "route-table" {
					app.Route.Tab = 1
				}
				app.Route.Net = "10.0.0.0"
				app.Route.Mask = "255.0.0.0"
				app.Route.MaskChoice = maskLabel("255.0.0.0")
				app.Route.NetChoice = "（手动填写）"
				app.Route.Gateway = "192.168.1.1"
				app.Route.Entries = []routing.RouteEntry{
					{Dest: "0.0.0.0", Mask: "0.0.0.0", Gateway: "192.168.1.1", Interface: "192.168.1.10", Metric: "25"},
					{Dest: "10.0.0.0", Mask: "255.0.0.0", Gateway: "192.168.1.1", Metric: "1", Persistent: true},
					{Dest: "172.16.0.0", Mask: "255.240.0.0", Gateway: "192.168.1.254", Metric: "5", Persistent: true},
				}
				app.computeRoute()
			}
			if tc.name == "log" {
				app.LogText = "===============================\n时间：2026-11-25 12:20:00\n项目地点：北京总部\n网卡：以太网\nIP地址：192.168.1.10\n子网掩码：255.255.255.0\n网关：192.168.1.1\n==============================="
			}
			tt := ui.NewTester(tc.view, tc.w, tc.h)
			img := tt.Image()
			if img == nil {
				t.Fatalf("%s: no frame", tc.name)
			}
			f, err := os.Create(filepath.Join(dir, tc.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
		}()
	}
}
