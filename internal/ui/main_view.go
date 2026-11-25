package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"netshift/internal/hostsys"
	"netshift/internal/macaddr"
	"netshift/internal/model"
)

// MainView draws the main window: the adapter, the computer, the MAC address
// and the IPv4 settings, on one responsive Fluent grid with the status bar under it.
func (a *App) MainView(c *ui.Context) {
	useTheme(c)
	ui.Column(c).Fill().Background(colBackground).Children(func() {
		ui.Row(c).Grow(1).Padding(windowPad).Gap(colGap).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).MinWidth(520).Gap(colGap).Children(func() {
				a.hardwarePanel(c)
				a.computerPanel(c)
				a.macPanel(c)
			})
			ui.Column(c).Width(382).Gap(colGap).Children(func() {
				a.ipv4Panel(c)
			})
		})
		a.mainStatusBar(c)
	})
	if a.ConfirmRestart {
		res := ui.AlertDialog(c, &a.ConfirmRestart, "确认重启计算机",
			"重启将在 5 秒后开始，请确保已保存正在编辑的工作。\n确定立即重启计算机吗？",
			"取消", "立即重启")
		if res == 1 {
			a.restartComputer()
		}
	}
	if a.Conflict.DialogOpen {
		detail := fmt.Sprintf("目标 IP %s 已被局域网内其他主机占用。", a.Conflict.Result.IP)
		if a.Conflict.Result.MAC != "" {
			detail += fmt.Sprintf("\n占用设备 MAC：%s（ARP 探测）", a.Conflict.Result.MAC)
		} else {
			detail += fmt.Sprintf("\n主机有 ICMP 应答（%d ms）", a.Conflict.Result.RTT.Milliseconds())
		}
		detail += "\n\n继续写入可能导致 IP 地址冲突，确定仍然应用吗？"
		res := ui.AlertDialog(c, &a.Conflict.DialogOpen, "检测到 IP 地址冲突", detail, "取消修改", "仍然应用")
		if res == 1 {
			a.executeApplyReq(a.Conflict.PendingReq, a.Conflict.PendingProfile)
		}
	}
}

func (a *App) mainStatusBar(c *ui.Context) {
	text := a.statusText()
	busy := a.busyNow()
	fg := colLabel
	switch {
	case strings.HasPrefix(text, "✗"):
		fg = colDanger
	case strings.HasPrefix(text, "✓"):
		fg = colSuccess
	case busy:
		fg = colAccent
	}
	ui.Row(c).Height(statusH).PaddingX(windowPad).Gap(8).AlignItems(ui.Center).
		Background(colStatusBar).
		BorderWidth(1, 0, 0, 0).BorderColor(colBorder).Children(func() {
		if busy {
			ui.Spinner(c).Size(13, 13)
		}
		ui.Text(c, text).SingleLine().Grow(1).FontSize(hintSize).TextColor(fg)
		if a.elevated {
			pill(c, "管理员", colInfo, colInfoBg, colInfoBorder)
		} else {
			pill(c, "标准权限", colWarn, colWarnBg, colWarnBorder)
		}
		v := strings.TrimPrefix(a.Version(), "v")
		ui.Text(c, "Crafted By OAyy").FontSize(hintSize).TextColor(colTextMuted).SingleLine()
		ui.Text(c, "|").FontSize(hintSize).TextColor(colBorder).SingleLine()
		ui.Text(c, "v"+v).FontSize(hintSize).TextColor(colTextMuted).SingleLine()
	})
}

func (a *App) hardwarePanel(c *ui.Context) {
	ad, hasAd := a.currentAdapterOK()
	headerRight := func() {
		if !hasAd {
			return
		}
		switch ad.Status() {
		case "已连接":
			pill(c, "已连接", colSuccess, colSuccessBg, colSuccessBorder)
		case "无 IP":
			pill(c, "无 IP", colWarn, colWarnBg, colWarnBorder)
		default:
			pill(c, "已断开", colDanger, colDangerBg, colDangerBorder)
		}
		if ad.DHCP {
			pill(c, "DHCP", colInfo, colInfoBg, colInfoBorder)
		} else {
			pill(c, "静态", colTextMuted, colSurfaceSubtle, colBorderSoft)
		}
		if spd := ad.SpeedLabel(); spd != "" && ad.Up {
			pill(c, spd, colTextMuted, colSurfaceSubtle, colBorderSoft)
		}
	}
	panelHeader(c, "选择硬件", 0, headerRight, func() {
		ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "网络适配器:").Width(labelW).FontSize(bodySize).TextColor(colLabel).SingleLine()
			sel := ui.Select(c, &a.adapterLabel, a.adapterNames()).Grow(1).Height(rowH - 2).FontSize(bodySize)
			if sel.Changed() {
				a.selectAdapterByLabel(a.adapterLabel)
				a.RefreshTrayMenu()
			}
			button(c, "刷新数据", 88, a.refreshData).Disabled(a.busyNow())
		})
		actionRow(c, func() {
			button(c, "启用网卡", 0, func() { a.toggleAdapter(true) }).Grow(1).Disabled(a.busyNow() || !hasAd)
			button(c, "禁用网卡", 0, func() { a.toggleAdapter(false) }).Grow(1).Disabled(a.busyNow() || !hasAd)
			button(c, "打开网络连接", 0, a.openNetworkConnections).Grow(1)
		})
		if a.adaptersErr != "" {
			note(c, "读取网卡失败："+a.adaptersErr)
		} else if hasAd {
			ui.Row(c).Height(24).Gap(rowGap).AlignItems(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprintf("MAC %s  ·  %s  ·  索引 %d  ·  DHCP %s",
					emptyDash(macaddr.Format(ad.MAC)), ad.TypeLabel(), ad.Index, yesNo(ad.DHCP))).
					FontSize(hintSize).TextColor(colTextMuted).SingleLine().Grow(1)
				quickChip(c, "复制信息", false, func() {
					c.WriteClipboard(a.currentSummary())
					a.setStatus("✓ 已复制当前网卡网络配置摘要")
				})
			})
		}
	})
}

func (a *App) computerPanel(c *ui.Context) {
	headerRight := func() {
		ui.Text(c, "修改后需重启生效").FontSize(hintSize).TextColor(colTextMuted).SingleLine()
	}
	panelHeader(c, "计算机信息", 0, headerRight, func() {
		pairRow(c, "计算机名:", func() {
			textField(c, &a.Form.Computer, 0).Grow(1).Placeholder("如 PC-0001")
		}, "工作组:", 52, func() {
			textField(c, &a.Form.Workgroup, 0).Grow(1).Placeholder("WORKGROUP")
		})
		actionRow(c, func() {
			button(c, "随机生成计算机名", 0, func() {
				a.Form.Computer = hostsys.RandomName()
				a.setStatus("已生成随机计算机名：" + a.Form.Computer)
			}).Grow(1)
			button(c, "修改计算机名", 0, a.renameComputer).Grow(1).Disabled(a.busyNow())
			button(c, "修改工作组", 0, a.joinWorkgroup).Grow(1).Disabled(a.busyNow())
			dangerButton(c, "重启电脑", 84, func() {
				a.ConfirmRestart = true
			}).Disabled(a.busyNow())
		})
		if a.hostErr != "" {
			note(c, "读取计算机信息失败："+a.hostErr)
		}
	})
}

func (a *App) macPanel(c *ui.Context) {
	ad, hasAd := a.currentAdapterOK()
	status := "未选择"
	if hasAd {
		status = ad.Status()
	}
	panel(c, "网卡 MAC 地址管理", 0, func() {
		pairRow(c, "MAC 地址:", func() {
			monoField(c, &a.Form.MAC, 0).Grow(1).Placeholder("AA-BB-CC-DD-EE-FF")
		}, "状态:", 52, func() {
			textField(c, &status, 0).Grow(1).ReadOnly(true)
		})
		actionRow(c, func() {
			button(c, "随机生成MAC地址", 0, func() {
				a.Form.MAC = macaddr.Format(macaddr.Random())
				a.setStatus("已生成随机单播 MAC：" + a.Form.MAC)
			}).Grow(1)
			button(c, "修改MAC地址", 0, a.setMAC).Grow(1).Disabled(a.busyNow() || !hasAd)
			button(c, "还原MAC地址", 0, a.restoreMAC).Grow(1).Disabled(a.busyNow() || !hasAd)
		})
		hint(c, "修改或还原后会短暂禁用并重新启用网卡，生效需要约 2~5 秒。")
	}).Grow(1)
}

func (a *App) ipv4Panel(c *ui.Context) {
	snap, hasSnap := a.currentSnapshot()
	headerRight := func() {
		btn := quickChip(c, "回退上次", !hasSnap || a.busyNow(), a.rollbackSnapshot)
		if hasSnap {
			btn.Tooltip(fmt.Sprintf("回退至 %s（%s 保存）", snap.Summary, snap.At.Format("15:04:05")))
		} else {
			btn.Tooltip("修改配置前会自动备份快照，届时可一键回退")
		}
	}
	panelHeader(c, "Internet 协议版本 4 (TCP/IPv4) 属性", 0, headerRight, func() {
		ui.RadioGroup(c, func() {
			ui.Radio(c, &a.Form.Mode, 0, "自动获取地址").FontSize(bodySize)
			ui.Radio(c, &a.Form.Mode, 1, "使用以下地址").FontSize(bodySize)
		}).Row().Gap(colGap)
		ui.Divider(c)
		auto := a.Form.Mode == 0
		field := func(label string, value *string, placeholder string) {
			formRowW(c, label, labelWNarrow, func() {
				monoField(c, value, 0).Grow(1).Disabled(auto).Placeholder(placeholder)
			})
		}
		field("IP 地址:", &a.Form.IP, "192.168.1.10")
		field("子网掩码:", &a.Form.Mask, "255.255.255.0")
		field("默认网关:", &a.Form.Gateway, "192.168.1.1")
		field("首选 DNS:", &a.Form.DNS1, "192.168.1.1")
		field("备用 DNS:", &a.Form.DNS2, "8.8.8.8")

		// Quick preset chips for mask & popular DNS servers.
		ui.Row(c).Height(24).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "快填:").FontSize(hintSize).TextColor(colLabel).SingleLine()
			quickChip(c, "/24", auto, func() { a.Form.Mask = "255.255.255.0" })
			quickChip(c, "/16", auto, func() { a.Form.Mask = "255.255.0.0" })
			quickChip(c, "阿里DNS", auto, func() { a.Form.DNS1 = "223.5.5.5"; a.Form.DNS2 = "223.6.6.6" })
			quickChip(c, "114DNS", auto, func() { a.Form.DNS1 = "114.114.114.114"; a.Form.DNS2 = "114.114.115.115" })
			quickChip(c, "Google", auto, func() { a.Form.DNS1 = "8.8.8.8"; a.Form.DNS2 = "8.8.4.4" })
		})

		// Conflict pre-check toggle & manual check shortcut.
		ui.Row(c).Height(24).Gap(rowGap).AlignItems(ui.Center).Children(func() {
			ui.Checkbox(c, &a.Conflict.Enabled, "应用前预检 IP 冲突").FontSize(hintSize).Disabled(auto)
			ui.Spacer(c).Grow(1)
			quickChip(c, "检测冲突", auto || a.busyNow(), a.checkConflictOnly)
		})

		if err := a.formError(); err != nil && !auto {
			note(c, err.Error())
		}
		ui.Divider(c)
		actionRow(c, func() {
			primaryButton(c, "应用配置", 0, a.applyNow).Grow(1).Disabled(a.busyNow())
			button(c, "恢复自动获取", 0, a.applyDHCP).Grow(1).Disabled(a.busyNow())
		})
		actionRow(c, func() {
			button(c, "导入配置", 0, a.openPresets).Grow(1)
			button(c, "保存当前配置", 0, a.saveCurrent).Grow(1)
		})
		actionRow(c, func() {
			button(c, "高级选项", 0, a.openAdvanced).Grow(1)
			button(c, "工具箱", 0, a.openToolbox).Grow(1)
		})
		ui.Spacer(c).Grow(1)
	}).Grow(1)
}

// formError reports the first validation error across IP, mask, gateway and DNS.
func (a *App) formError() error {
	if a.Form.Mode == 0 {
		return nil
	}
	p := a.Form.Profile("check", "", a.adapter)
	if len(p.IPv4) == 0 && len(p.Masks) == 0 && len(p.Gateways) == 0 && len(p.DNS) == 0 {
		return nil
	}
	if len(p.IPv4) == 0 {
		for i, g := range p.Gateways {
			if err := model.ValidateIPv4(g.Addr); err != nil {
				return fmt.Errorf("第 %d 个网关：%w", i+1, err)
			}
		}
		for i, d := range p.DNS {
			if err := model.ValidateIPv4(d); err != nil {
				return fmt.Errorf("第 %d 个 DNS：%w", i+1, err)
			}
		}
		return nil
	}
	return p.Validate()
}

// AboutView draws the about window with the same Fluent header & statusBar.
func (a *App) AboutView(c *ui.Context) {
	useTheme(c)
	headerRight := func() {
		pill(c, "v"+strings.TrimPrefix(a.Version(), "v"), colInfo, colInfoBg, colInfoBorder)
	}
	ui.Column(c).Fill().Background(colBackground).Children(func() {
		ui.Column(c).Grow(1).Padding(windowPad).Gap(colGap).Children(func() {
			panelHeader(c, "NetShift · 网络配置工具箱", 0, headerRight, func() {
				ui.Text(c, "灵感源于 kn007 的 IP 地址修改器 5.0 重制版，").FontSize(bodySize)
				ui.Text(c, "融合 mulNetworkTest.bat 的项目地点继承、内网分流路由、连通性诊断与代理/Hosts 管理。").FontSize(bodySize)
				ui.Divider(c)
				ui.Text(c, "Crafted By OAyy").FontSize(hintSize).TextColor(colLabel)
				ui.Text(c, "Original Made By kn007 · Blog: kn007.net").FontSize(hintSize).TextColor(colTextMuted)
				ui.Text(c, "技术栈：Go 1.27 + mygo 原生 GPU 渲染 UI").FontSize(hintSize).TextColor(colTextMuted)
			}).Grow(1)
		})
		statusBar(c, "就绪", a.Version(), false)
	})
}

func yesNo(b bool) string {
	if b {
		return "是"
	}
	return "否"
}
