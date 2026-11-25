package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// ToolboxView draws the toolbox window as clean descriptive action cards.
func (a *App) ToolboxView(c *ui.Context) {
	useTheme(c)
	toolCard := func(title, desc string, onClick func()) {
		card := ui.ButtonBase(c).PaddingX(12).PaddingY(9).
			Background(colSurfaceSubtle).Border(1, colBorderSoft).Radius(ctrlRadius).
			AlignItems(ui.Center)
		if card.Hovered() {
			card = card.Background(colSurfaceHover).Border(1, colInfoBorder)
		}
		card.OnClick(onClick).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, title).FontSize(bodySize).FontWeight(weightHeading).TextColor(colText).SingleLine()
				ui.Text(c, desc).FontSize(hintSize).TextColor(colTextMuted).SingleLine()
			})
			ui.Text(c, "›").FontSize(18).TextColor(colTextMuted)
		})
	}
	ui.Column(c).Fill().Background(colBackground).FontWeight(weightRegular).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(windowPad).Gap(rowGap).Children(func() {
				panel(c, "工具箱", 0, func() {
					toolCard("C网群Ping器", "并发 ICMP 扫描 /24 网段，支持空闲 IP 一键填入主界面", a.openPing)
					toolCard("网络连通性诊断", "一键并发检测网关、DNS 解析、公网与自定义 TCP 端口", a.openDiag)
					toolCard("子网掩码计算器", "支持 CIDR 输入、掩码换算与可用主机范围", a.openSubnet)
					toolCard("内网分流路由", "删除默认路由、配置静态分流及可视化删除已有路由", a.openRoute)
					toolCard("代理与 Hosts 管理", "一键开关 Windows 系统代理与快捷编辑 Hosts 映射", a.openSysNet)
					toolCard("操作日志", "查看历史 IP 与分流路由变更记录", a.openLog)
				})
			})
		})
		footerBar(c, func() {
			button(c, "打开网络连接", 0, a.openNetworkConnections).Grow(1)
			button(c, "关于", 0, a.openAbout).Grow(1)
		})
		statusBar(c, a.statusText(), a.Version(), a.busyNow())
	})
}

// SubnetView draws the subnet mask calculator with zero overlapping controls,
// CIDR quick-entry, common prefix chips and unified footerBar.
func (a *App) SubnetView(c *ui.Context) {
	useTheme(c)
	setPrefix := func(p int) {
		if p < 0 {
			p = 0
		}
		if p > 32 {
			p = 32
		}
		a.Sub.Prefix = float64(p)
		a.Sub.PrefixText = strconv.Itoa(p)
		a.recalcSubnet()
	}
	ui.Column(c).Fill().Background(colBackground).FontWeight(weightRegular).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(windowPad).Gap(colGap).Children(func() {
				panel(c, "主机信息", 0, func() {
					formRow(c, "IP / CIDR:", func() {
						if monoField(c, &a.Sub.IP, 0).Grow(1).Placeholder("如 192.168.1.10 或 192.168.1.10/24").Changed() {
							a.recalcSubnet()
						}
					})
					formRow(c, "前缀长度:", func() {
						button(c, "-", 30, func() { setPrefix(int(a.Sub.Prefix) - 1) })
						if monoField(c, &a.Sub.PrefixText, 54).Changed() {
							if n, err := strconv.Atoi(strings.TrimSpace(a.Sub.PrefixText)); err == nil && n >= 0 && n <= 32 {
								a.Sub.Prefix = float64(n)
								a.recalcSubnet()
							}
						}
						button(c, "+", 30, func() { setPrefix(int(a.Sub.Prefix) + 1) })
						pill(c, fmt.Sprintf("/%d", int(a.Sub.Prefix)), colInfo, colInfoBg, colInfoBorder)
						ui.Spacer(c).Grow(1)
						ui.Text(c, a.Sub.Info.Mask).FontSize(bodySize).TextColor(colText).SingleLine()
					})
					ui.Slider(c, &a.Sub.Prefix, 0, 32).Grow(1).OnChange(func() {
						setPrefix(int(a.Sub.Prefix + 0.5))
					})
					ui.Row(c).Height(24).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "常用前缀:").FontSize(hintSize).TextColor(colLabel).SingleLine()
						for _, p := range []int{8, 16, 20, 24, 25, 26, 30} {
							p := p
							quickChip(c, fmt.Sprintf("/%d", p), false, func() { setPrefix(p) })
						}
					})
					note(c, a.Sub.Err)
				})
				panel(c, "计算结果", 0, func() {
					result := func(label, value string) {
						ui.Row(c).Height(24).Gap(rowGap).AlignItems(ui.Center).Children(func() {
							ui.Text(c, label).Width(labelW).FontSize(bodySize).TextColor(colLabel).SingleLine()
							ui.Text(c, emptyDash(value)).TextColor(colText).Grow(1).FontSize(bodySize).SingleLine()
						})
					}
					result("子网掩码:", a.Sub.Info.Mask)
					result("网络地址:", a.Sub.Info.Network)
					result("广播地址:", a.Sub.Info.Broadcast)
					result("起始地址:", a.Sub.Info.First)
					result("结束地址:", a.Sub.Info.Last)
					result("可用主机数:", fmt.Sprintf("%d", a.Sub.Info.Hosts))
					result("地址总数:", fmt.Sprintf("%d", a.Sub.Info.Total))
					ui.Divider(c)
					hint(c, "支持直接输入 CIDR（如 10.0.0.1/22）；/31 计 2 个可用地址，/32 计 1 个。")
				})
			})
		})
		footerBar(c, func() {
			button(c, "填入主界面掩码", 140, func() {
				if a.Sub.Info.Mask == "" {
					a.setStatus("✗ 当前没有有效子网掩码")
					return
				}
				a.Form.Mask = a.Sub.Info.Mask
				a.Form.Mode = 1
				a.setStatus("✓ 已将掩码 " + a.Sub.Info.Mask + " 填入主界面")
			})
			button(c, "复制主机范围", 126, func() {
				text := a.Sub.Info.Range()
				if text == "" {
					a.setStatus("✗ 没有可复制的主机范围")
					return
				}
				c.WriteClipboard(text)
				a.setStatus("✓ 已复制主机范围 " + text)
			})
			ui.Spacer(c).Grow(1)
			button(c, "退出到主界面", 126, func() { a.closeWindow("subnet") })
		})
		statusBar(c, a.statusText(), a.Version(), a.busyNow())
	})
}

// PingView draws the C-class ping screen with compact 254-cell grid, host
// selection for one-click "设为静态 IP", plus unified footerBar & statusBar.
func (a *App) PingView(c *ui.Context) {
	useTheme(c)
	headerRight := func() {
		pill(c, fmt.Sprintf("进度 %d/254", a.Ping.Done), colInfo, colInfoBg, colInfoBorder)
		pill(c, fmt.Sprintf("在线 %d", a.Ping.Online), colSuccess, colSuccessBg, colSuccessBorder)
	}
	ui.Column(c).Fill().Background(colBackground).FontWeight(weightRegular).Children(func() {
		ui.Column(c).Grow(1).Padding(windowPad).Gap(rowGap).Children(func() {
			panelHeader(c, "探测设置", 0, headerRight, func() {
				ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "网段:").Width(42).FontSize(bodySize).TextColor(colLabel).SingleLine()
					monoField(c, &a.Ping.Prefix, 138).Placeholder("192.168.1").Disabled(a.Ping.Running)
					ui.Text(c, "并发:").Width(40).FontSize(bodySize).TextColor(colLabel).SingleLine()
					monoField(c, &a.Ping.WorkersText, 56).Placeholder("32").Disabled(a.Ping.Running)
					ui.Text(c, "超时(ms):").Width(68).FontSize(bodySize).TextColor(colLabel).SingleLine()
					monoField(c, &a.Ping.TimeoutText, 68).Placeholder("1000").Disabled(a.Ping.Running)
					ui.Spacer(c).Grow(1)
					button(c, "按当前网卡填入", 140, func() {
						if ip := a.currentAdapter().CurrentIP(); ip != "" {
							if i := lastDot(ip); i > 0 {
								a.Ping.Prefix = ip[:i]
								a.setStatus("已填入当前网卡网段 " + a.Ping.Prefix)
							}
						} else {
							a.setStatus("✗ 当前网卡暂无 IPv4 地址")
						}
					}).Disabled(a.Ping.Running)
				})
				if a.Ping.Running || a.Ping.Done > 0 {
					ui.Progress(c, float64(a.Ping.Done)/254.0).Height(5).Grow(1)
				}
			})
			panel(c, "探测结果（点击主机号可单点重测，并可设为静态 IP）", 0, func() {
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Grid(c).Columns(16).Gap(2).Children(func() {
						for n := 1; n <= 254; n++ {
							n := n
							bg := colUnknown
							fg := colText
							bdr := colBorderSoft
							bdrW := float32(1)
							switch a.Ping.State[n] {
							case 1:
								bg = colOnline
								fg = colAccentText
								bdr = colOnlineBorder
							case 2:
								bg = colOffline
								fg = colOfflineText
								bdr = colBorderSoft
							}
							if a.Ping.SelectedHost == n {
								bdr = colAccent
								bdrW = 2
							}
							cell := ui.ButtonBase(c.Key(n)).Size(40, 22).
								Background(bg).Border(bdrW, bdr).Radius(ctrlRadius).
								AlignItems(ui.Center).Justify(ui.Center)
							if cell.Hovered() && a.Ping.State[n] != 1 {
								cell = cell.Background(colSurfaceHover).Border(bdrW, colAccent)
							}
							if a.Ping.State[n] == 1 {
								cell = cell.Tooltip(fmt.Sprintf("%s.%d 在线 · %d ms", a.Ping.Prefix, n, a.Ping.RTT[n]))
							} else if a.Ping.State[n] == 2 {
								cell = cell.Tooltip(fmt.Sprintf("%s.%d 无应答（点击选中并重测，可设为静态 IP）", a.Ping.Prefix, n))
							} else {
								cell = cell.Tooltip(fmt.Sprintf("点击选中并探测 %s.%d", a.Ping.Prefix, n))
							}
							cell.OnClick(func() {
								a.Ping.SelectedHost = n
								a.setStatus(fmt.Sprintf("正在探测 %s.%d …", a.Ping.Prefix, n))
								go a.probeOne(n)
							}).Children(func() {
								ui.Text(c, strconv.Itoa(n)).FontSize(hintSize).TextColor(fg).SingleLine()
							})
						}
					})
				})
				ui.Row(c).Height(20).Gap(colGap).AlignItems(ui.Center).Children(func() {
					legend(c, colOnline, "在线")
					legend(c, colOffline, "无应答")
					legend(c, colUnknown, "未探测")
					ui.Spacer(c).Grow(1)
					if a.Ping.SelectedHost > 0 {
						hint(c, fmt.Sprintf("已选 %s.%d  ·  进度 %d/254 · 在线 %d", a.Ping.Prefix, a.Ping.SelectedHost, a.Ping.Done, a.Ping.Online))
					} else {
						hint(c, fmt.Sprintf("进度 %d/254 · 在线 %d", a.Ping.Done, a.Ping.Online))
					}
				})
			}).Grow(1)
		})
		footerBar(c, func() {
			if a.Ping.Running {
				dangerButton(c, "停止探测", 104, a.stopPing)
			} else {
				primaryButton(c, "开始探测", 104, a.startPing)
			}
			button(c, "复制在线 IP", 112, func() {
				ips := a.onlineIPs()
				if len(ips) == 0 {
					a.setStatus("✗ 暂无在线主机可复制")
					return
				}
				c.WriteClipboard(strings.Join(ips, "\n"))
				a.setStatus(fmt.Sprintf("✓ 已复制 %d 个在线 IP 到剪贴板", len(ips)))
			})
			useLabel := "设为静态 IP"
			if a.Ping.SelectedHost > 0 {
				useLabel = fmt.Sprintf("设为静态 IP (.%d)", a.Ping.SelectedHost)
			}
			button(c, useLabel, 148, a.useSelectedPingIP).Disabled(a.Ping.SelectedHost <= 0)
			ui.Spacer(c).Grow(1)
			button(c, "关闭", 80, func() { a.closeWindow("ping") })
		})
		statusBar(c, a.statusText(), a.Version(), a.Ping.Running)
	})
}

// LogView shows the recent entries of network_config.log with unified chrome.
func (a *App) LogView(c *ui.Context) {
	useTheme(c)
	ui.Column(c).Fill().Background(colBackground).FontWeight(weightRegular).Children(func() {
		ui.Column(c).Grow(1).Padding(windowPad).Gap(rowGap).Children(func() {
			panelHeader(c, "操作日志（network_config.log）", 0, func() {
				quickChip(c, "刷新", false, func() {
					a.reloadLog()
					a.setStatus("✓ 日志已刷新")
				})
				quickChip(c, "复制全部", false, func() {
					if a.LogText == "" {
						a.setStatus("✗ 日志为空")
						return
					}
					c.WriteClipboard(a.LogText)
					a.setStatus("✓ 已复制操作日志到剪贴板")
				})
			}, func() {
				ui.Text(c, "路径："+a.store.LogPath()).FontSize(hintSize).TextColor(colTextMuted).SingleLine()
				ui.Scroll(c).Grow(1).Background(colCodeBg).Radius(ctrlRadius).Padding(10).Children(func() {
					ui.Column(c).Gap(2).Children(func() {
						if strings.TrimSpace(a.LogText) == "" {
							ui.Text(c, "暂无操作日志记录。应用 IP 配置或内网分流路由后会自动写入此处。").
								Font(monoFont).FontSize(hintSize).TextColor(colCodeMuted)
							return
						}
						for _, line := range strings.Split(a.LogText, "\n") {
							ui.Text(c, line).Font(monoFont).FontSize(hintSize).TextColor(colCodeText).SingleLine()
						}
					})
				})
			}).Grow(1)
		})
		footerBar(c, func() {
			button(c, "刷新日志", 104, func() {
				a.reloadLog()
				a.setStatus("✓ 日志已刷新")
			})
			button(c, "复制日志", 104, func() {
				if a.LogText == "" {
					a.setStatus("✗ 日志为空")
					return
				}
				c.WriteClipboard(a.LogText)
				a.setStatus("✓ 已复制操作日志到剪贴板")
			})
			ui.Spacer(c).Grow(1)
			button(c, "关闭", 84, func() { a.closeWindow("log") })
		})
		statusBar(c, a.statusText(), a.Version(), a.busyNow())
	})
}

// legend draws one colour of the ping grid with its meaning.
func legend(c *ui.Context, color ui.Color, label string) {
	ui.Row(c).Gap(unit).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(12, 12).Background(color).Border(1, colBorder).Radius(2)
		ui.Text(c, label).FontSize(hintSize).TextColor(colTextMuted)
	})
}

// probeOne pings a single host of the screen's network.
func (a *App) probeOne(n int) {
	prefix := strings.TrimSuffix(strings.TrimSpace(a.Ping.Prefix), ".")
	if prefix == "" {
		a.setStatus("✗ 请先填写网段前缀")
		return
	}
	ip := fmt.Sprintf("%s.%d", prefix, n)
	r := pingOne(ip, a.Ping.Timeout)
	a.post(func() {
		prevOnline := a.Ping.State[n] == 1
		if r.Online {
			a.Ping.State[n] = 1
			a.Ping.RTT[n] = r.RTT
			if !prevOnline {
				a.Ping.Online++
			}
			a.setStatus(fmt.Sprintf("✓ %s 在线（%d ms）", ip, r.RTT))
		} else {
			a.Ping.State[n] = 2
			if prevOnline && a.Ping.Online > 0 {
				a.Ping.Online--
			}
			a.setStatus(fmt.Sprintf("✗ %s 无应答（可点击「设为静态 IP」使用）", ip))
		}
	})
}
