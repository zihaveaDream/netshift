package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"netshift/internal/model"
)

// AdvancedView draws the advanced options with the same Fluent card header,
// unified footer bar and status bar used across all other windows.
func (a *App) AdvancedView(c *ui.Context) {
	useTheme(c)
	if a.Adv.Tab != a.Adv.LastTab {
		a.Adv.Draft = AddressRow{}
		a.Adv.LastTab = a.Adv.Tab
	}
	tabTitles := []string{"IP 地址列表", "网关地址列表", "DNS 服务器列表", "跃点数与系统维护"}
	tabTitle := tabTitles[0]
	if a.Adv.Tab >= 0 && a.Adv.Tab < len(tabTitles) {
		tabTitle = tabTitles[a.Adv.Tab]
	}
	ad, hasAd := a.currentAdapterOK()
	headerRight := func() {
		if hasAd {
			pill(c, ad.Name, colInfo, colInfoBg, colInfoBorder)
		}
	}
	ui.Column(c).Fill().Background(colBackground).FontWeight(weightRegular).Children(func() {
		ui.Column(c).Grow(1).Padding(windowPad).Gap(rowGap).Children(func() {
			ui.Tabs(c, &a.Adv.Tab, "IP 地址", "网关地址", "DNS 服务器", "其他功能").Height(34).FontSize(bodySize)
			panelHeader(c, tabTitle, 0, headerRight, func() {
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Gap(rowGap).Children(func() {
						switch a.Adv.Tab {
						case 0:
							a.advancedIPs(c)
						case 1:
							a.advancedGateways(c)
						case 2:
							a.advancedDNS(c)
						default:
							a.advancedOther(c)
						}
					})
				})
			}).Grow(1)
		})
		footerBar(c, func() {
			hint(c, "「仅应用本页设置」仅写入当前页；「应用」应用全部配置")
			ui.Spacer(c).Grow(1)
			button(c, "仅应用本页设置", 148, func() { a.advancedApply(true) }).Disabled(a.busyNow() || a.Adv.Tab == 3)
			primaryButton(c, "应用", 96, func() { a.advancedApply(false) }).Disabled(a.busyNow())
			button(c, "返回", 84, func() { a.closeWindow("advanced") })
		})
		statusBar(c, a.statusText(), a.Version(), a.busyNow())
	})
}

func (a *App) advancedIPs(c *ui.Context) {
	hint(c, "一份配置可以带多个 IP 地址，第 1 个作为主地址同步至主界面。")
	tableHeader(c, headerCol{"地址", colIP}, headerCol{"掩码", colMask})
	ui.Column(c).Gap(unit).Children(func() {
		total := len(a.Adv.IPs)
		for i := range a.Adv.IPs {
			i := i
			key := fmt.Sprintf("ip-row-%d", i)
			listRow(c, key, i, total, func() {
				inlineField(c, &a.Adv.IPs[i].Value, colIP, rowH-4, bodySize).Placeholder("192.168.1.10")
				inlineField(c, &a.Adv.IPs[i].Mask, colMask, rowH-4, bodySize).Placeholder("255.255.255.0")
			},
				func() { a.moveRow(a.Adv.IPs, i, -1) },
				func() { a.moveRow(a.Adv.IPs, i, +1) },
				func() { a.Adv.IPs = append(a.Adv.IPs[:i], a.Adv.IPs[i+1:]...) },
			)
			if i < len(a.Adv.IPs) {
				row := a.Adv.IPs[i]
				if err := model.ValidateIPv4(row.Value); row.Value != "" && err != nil {
					note(c, err.Error())
				} else if err := model.ValidateMask(row.Mask); row.Mask != "" && err != nil {
					note(c, err.Error())
				}
			}
		}
	})
	ui.Divider(c)
	ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
		inlineField(c, &a.Adv.Draft.Value, colIP, rowH-2, bodySize).Placeholder("新增 IP 地址")
		inlineField(c, &a.Adv.Draft.Mask, colMask, rowH-2, bodySize).Placeholder("255.255.255.0")
		button(c, "添加", 84, func() {
			ip := strings.TrimSpace(a.Adv.Draft.Value)
			if ip == "" {
				a.setStatus("✗ 请先填写 IP 地址")
				return
			}
			if err := model.ValidateIPv4(ip); err != nil {
				a.setStatus("✗ " + err.Error())
				return
			}
			mask := strings.TrimSpace(a.Adv.Draft.Mask)
			if mask == "" {
				mask = "255.255.255.0"
			}
			if err := model.ValidateMask(mask); err != nil {
				a.setStatus("✗ " + err.Error())
				return
			}
			a.Adv.IPs = append(a.Adv.IPs, AddressRow{Value: ip, Mask: mask})
			a.Adv.Draft = AddressRow{}
			a.setStatus("✓ 已添加 IP " + ip)
		})
	})
}

func (a *App) advancedGateways(c *ui.Context) {
	hint(c, "网关与跃点数，第一个网关写入主界面；跃点数留空表示由系统决定。")
	tableHeader(c, headerCol{"网关", colGW}, headerCol{"跃点数", colMetric})
	ui.Column(c).Gap(unit).Children(func() {
		total := len(a.Adv.GWs)
		for i := range a.Adv.GWs {
			i := i
			key := fmt.Sprintf("gw-row-%d", i)
			listRow(c, key, i, total, func() {
				inlineField(c, &a.Adv.GWs[i].Value, colGW, rowH-4, bodySize).Placeholder("192.168.1.1")
				inlineField(c, &a.Adv.GWs[i].Metric, colMetric, rowH-4, bodySize).Placeholder("自动")
			},
				func() { a.moveRow(a.Adv.GWs, i, -1) },
				func() { a.moveRow(a.Adv.GWs, i, +1) },
				func() { a.Adv.GWs = append(a.Adv.GWs[:i], a.Adv.GWs[i+1:]...) },
			)
			if i < len(a.Adv.GWs) {
				row := a.Adv.GWs[i]
				if err := model.ValidateIPv4(row.Value); row.Value != "" && err != nil {
					note(c, err.Error())
				} else if ms := strings.TrimSpace(row.Metric); ms != "" {
					if m, err := strconv.Atoi(ms); err != nil || m < 0 || m > 9999 {
						note(c, "跃点数应为 0..9999 的整数："+ms)
					}
				}
			}
		}
	})
	ui.Divider(c)
	ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
		inlineField(c, &a.Adv.Draft.Value, colGW, rowH-2, bodySize).Placeholder("新增网关地址")
		inlineField(c, &a.Adv.Draft.Metric, colMetric, rowH-2, bodySize).Placeholder("跃点")
		button(c, "添加", 84, func() {
			gw := strings.TrimSpace(a.Adv.Draft.Value)
			if gw == "" {
				a.setStatus("✗ 请先填写网关地址")
				return
			}
			if err := model.ValidateIPv4(gw); err != nil {
				a.setStatus("✗ " + err.Error())
				return
			}
			ms := strings.TrimSpace(a.Adv.Draft.Metric)
			if ms != "" {
				if m, err := strconv.Atoi(ms); err != nil || m < 0 || m > 9999 {
					a.setStatus("✗ 跃点数应为 0..9999 的整数")
					return
				}
			}
			a.Adv.GWs = append(a.Adv.GWs, AddressRow{Value: gw, Metric: ms})
			a.Adv.Draft = AddressRow{}
			a.setStatus("✓ 已添加网关 " + gw)
		})
	})
}

func (a *App) advancedDNS(c *ui.Context) {
	hint(c, "DNS 服务器按顺序写入：第 1 个为首选，其余为备用。")
	tableHeader(c, headerCol{"DNS", colDNS})
	ui.Column(c).Gap(unit).Children(func() {
		total := len(a.Adv.DNS)
		for i := range a.Adv.DNS {
			i := i
			key := fmt.Sprintf("dns-row-%d", i)
			listRow(c, key, i, total, func() {
				inlineField(c, &a.Adv.DNS[i].Value, colDNS, rowH-4, bodySize).Placeholder("8.8.8.8")
			},
				func() { a.moveRow(a.Adv.DNS, i, -1) },
				func() { a.moveRow(a.Adv.DNS, i, +1) },
				func() { a.Adv.DNS = append(a.Adv.DNS[:i], a.Adv.DNS[i+1:]...) },
			)
			if i < len(a.Adv.DNS) {
				row := a.Adv.DNS[i]
				if err := model.ValidateIPv4(row.Value); row.Value != "" && err != nil {
					note(c, err.Error())
				}
			}
		}
	})
	ui.Divider(c)
	ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
		inlineField(c, &a.Adv.Draft.Value, colDNS, rowH-2, bodySize).Placeholder("新增 DNS 服务器")
		button(c, "添加", 84, func() {
			d := strings.TrimSpace(a.Adv.Draft.Value)
			if d == "" {
				a.setStatus("✗ 请先填写 DNS 服务器")
				return
			}
			if err := model.ValidateIPv4(d); err != nil {
				a.setStatus("✗ " + err.Error())
				return
			}
			a.Adv.DNS = append(a.Adv.DNS, AddressRow{Value: d})
			a.Adv.Draft = AddressRow{}
			a.setStatus("✓ 已添加 DNS " + d)
		})
	})
}

// moveRow swaps an entry with its neighbour, which the ↑ and ↓ buttons do.
func (a *App) moveRow(rows []AddressRow, i, delta int) {
	j := i + delta
	if j < 0 || j >= len(rows) {
		return
	}
	rows[i], rows[j] = rows[j], rows[i]
}

func (a *App) advancedOther(c *ui.Context) {
	ui.Text(c, "修改适配器跃点数").FontSize(titleSize).FontWeight(weightHeading).TextColor(colText)
	ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
		ui.RadioGroup(c, func() {
			ui.Radio(c, &a.Adv.MetricAuto, true, "自动").FontSize(bodySize)
			ui.Radio(c, &a.Adv.MetricAuto, false, "自定义").FontSize(bodySize)
		}).Row().Gap(colGap)
		inlineField(c, &a.Adv.MetricCustom, 96, rowH-2, bodySize).
			Disabled(a.Adv.MetricAuto).Placeholder("1..9999")
		hint(c, "跃点数越小，多网卡同时在线时优先级越高")
	})
	ui.Divider(c)
	ui.Text(c, "IP 与 DNS 维护").FontSize(titleSize).FontWeight(weightHeading).TextColor(colText)
	ui.Column(c).Gap(rowGap).Children(func() {
		actionRow(c, func() {
			button(c, "释放适配器的 IP 地址", 0, a.releaseIP).Grow(1).Disabled(a.busyNow())
			button(c, "更新适配器的 IP 地址", 0, a.renewIP).Grow(1).Disabled(a.busyNow())
		})
		actionRow(c, func() {
			button(c, "清除 DNS 解析程序缓存", 0, a.flushDNS).Grow(1).Disabled(a.busyNow())
			button(c, "打开网络连接", 0, a.openNetworkConnections).Grow(1)
		})
	})
	ui.Divider(c)
	hint(c, "提示：「释放 / 更新 IP」仅对启用了 DHCP 自动获取的网卡生效。")
}
