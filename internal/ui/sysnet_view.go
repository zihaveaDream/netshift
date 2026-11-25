package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"netshift/internal/sysnet"
)

// SysNetView draws the System Proxy & Hosts file manager window using the same
// unified Fluent chrome as AdvancedView.
func (a *App) SysNetView(c *ui.Context) {
	useTheme(c)
	ui.Column(c).Fill().Background(colBackground).FontWeight(weightRegular).Children(func() {
		ui.Column(c).Grow(1).Padding(windowPad).Gap(rowGap).Children(func() {
			ui.Tabs(c, &a.SysNet.Tab, "系统代理开关", "Hosts 快捷管理").Height(34).FontSize(bodySize)
			if a.SysNet.Tab == 0 {
				a.proxyPanel(c)
			} else {
				a.hostsPanel(c)
			}
		})
		if a.SysNet.Tab == 0 {
			footerBar(c, func() {
				button(c, "刷新读取", 100, func() {
					a.loadSysNet()
					a.setStatus("✓ 已重新读取系统代理状态")
				})
				dangerButton(c, "一键关闭代理", 122, func() {
					cfg := a.SysNet.Proxy
					cfg.Enabled = false
					a.saveProxy(cfg)
				}).Disabled(a.busyNow())
				ui.Spacer(c).Grow(1)
				primaryButton(c, "保存并立即生效", 140, func() {
					a.saveProxy(a.SysNet.Proxy)
				}).Disabled(a.busyNow())
				button(c, "关闭", 80, func() { a.closeWindow("sysnet") })
			})
		} else {
			footerBar(c, func() {
				button(c, "重新读取", 100, func() {
					a.loadSysNet()
					a.setStatus("✓ 已重新读取系统 hosts 文件")
				})
				ui.Spacer(c).Grow(1)
				primaryButton(c, "保存并刷新 DNS", 148, a.saveHosts).Disabled(a.busyNow())
				button(c, "关闭", 80, func() { a.closeWindow("sysnet") })
			})
		}
		statusBar(c, a.statusText(), a.Version(), a.busyNow())
	})
}

func (a *App) proxyPanel(c *ui.Context) {
	headerRight := func() {
		if a.SysNet.Proxy.Enabled {
			pill(c, "代理已启用", colSuccess, colSuccessBg, colSuccessBorder)
		} else {
			pill(c, "代理已关闭", colTextMuted, colSurfaceSubtle, colBorderSoft)
		}
	}
	panelHeader(c, "Windows 系统代理（WinINet）", 0, headerRight, func() {
		ui.Checkbox(c, &a.SysNet.Proxy.Enabled, "启用系统代理服务器（保存后立即通知浏览器与系统生效）").FontSize(bodySize)
		ui.Divider(c)
		formRow(c, "代理地址:", func() {
			textField(c, &a.SysNet.Proxy.Server, 0).Grow(1).Placeholder("如 127.0.0.1:7890")
		})
		formRow(c, "忽略地址:", func() {
			textField(c, &a.SysNet.Proxy.Override, 0).Grow(1).Placeholder("localhost;127.*;10.*;192.168.*;<local>")
		})
		ui.Row(c).Height(24).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "快捷填值:").FontSize(hintSize).TextColor(colLabel).SingleLine()
			quickChip(c, "本地 :7890", false, func() {
				a.SysNet.Proxy.Enabled = true
				a.SysNet.Proxy.Server = "127.0.0.1:7890"
				if a.SysNet.Proxy.Override == "" {
					a.SysNet.Proxy.Override = "localhost;127.*;10.*;172.16.*;192.168.*;<local>"
				}
			})
			quickChip(c, "本地 :10809", false, func() {
				a.SysNet.Proxy.Enabled = true
				a.SysNet.Proxy.Server = "127.0.0.1:10809"
			})
			quickChip(c, "补全内网忽略", false, func() {
				a.SysNet.Proxy.Override = "localhost;127.*;10.*;172.16.*;192.168.*;<local>"
			})
		})
		note(c, a.SysNet.ProxyErr)
		ui.Divider(c)
		hint(c, "排查内网断网或切换环境后浏览器打不开网页时，可点下方「一键关闭代理」快速复位。")
		ui.Spacer(c).Grow(1)
	}).Grow(1)
}

func (a *App) hostsPanel(c *ui.Context) {
	total := 0
	if a.SysNet.Hosts != nil {
		total = len(a.SysNet.Hosts.Entries)
	}
	headerRight := func() {
		pill(c, fmt.Sprintf("%d 条映射", total), colInfo, colInfoBg, colInfoBorder)
	}
	panelHeader(c, "系统 Hosts 映射管理（保留原注释，首次保存自动备份）", 0, headerRight, func() {
		ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
			searchField(c, &a.SysNet.HostsQuery, "按 IP、域名或备注搜索").Grow(1)
			hint(c, sysnet.DefaultHostsPath())
		})
		note(c, a.SysNet.HostsErr)
		ui.Row(c).Height(24).PaddingX(4).Gap(rowGap).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "启用").Width(42).FontSize(hintSize).FontWeight(weightHeading).TextColor(colLabel)
			ui.Text(c, "IP 地址").Width(142).FontSize(hintSize).FontWeight(weightHeading).TextColor(colLabel)
			ui.Text(c, "域名").Grow(1).FontSize(hintSize).FontWeight(weightHeading).TextColor(colLabel)
			ui.Text(c, "备注").Width(120).FontSize(hintSize).FontWeight(weightHeading).TextColor(colLabel)
			ui.Box(c).Width(60)
		})
		ui.Scroll(c).Grow(1).Children(func() {
			if a.SysNet.Hosts == nil || len(a.SysNet.Hosts.Entries) == 0 {
				hint(c, "当前 hosts 文件没有可切换的映射条目，可在下方直接新增。")
				return
			}
			q := strings.ToLower(strings.TrimSpace(a.SysNet.HostsQuery))
			ui.Column(c).Gap(4).Children(func() {
				for i := range a.SysNet.Hosts.Entries {
					i := i
					e := &a.SysNet.Hosts.Entries[i]
					if q != "" &&
						!strings.Contains(strings.ToLower(e.IP), q) &&
						!strings.Contains(strings.ToLower(e.Host), q) &&
						!strings.Contains(strings.ToLower(e.Comment), q) {
						continue
					}
					ui.Row(c.Key(fmt.Sprintf("host-%d", i))).Height(rowH).PaddingX(4).Gap(rowGap).
						AlignItems(ui.Center).Background(colSurfaceSubtle).Border(1, colBorderSoft).Radius(ctrlRadius).Children(func() {
						ui.Box(c).Width(42).AlignItems(ui.Center).Children(func() {
							ui.Checkbox(c, &e.Enabled, "")
						})
						inlineField(c, &e.IP, 142, rowH-6, hintSize)
						inlineField(c, &e.Host, 0, rowH-6, hintSize).Grow(1)
						inlineField(c, &e.Comment, 120, rowH-6, hintSize).Placeholder("备注")
						ui.Button(c, "删除").Size(60, rowH-6).FontSize(hintSize).TextColor(colDanger).OnClick(func() {
							a.SysNet.Hosts.Delete(i)
							a.setStatus("已移除条目，点击「保存并刷新 DNS」生效")
						})
					})
				}
			})
		})
		ui.Divider(c)
		ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
			textField(c, &a.SysNet.NewHostIP, 148).Placeholder("新增 IP，如 127.0.0.1")
			textField(c, &a.SysNet.NewHostDomain, 0).Grow(1).Placeholder("域名，如 api.local")
			textField(c, &a.SysNet.NewHostNote, 120).Placeholder("备注（可选）")
			button(c, "添加", 76, a.addHostEntry)
		})
	}).Grow(1)
}
