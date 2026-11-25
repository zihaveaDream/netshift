package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netshift/internal/pingfan"
	"netshift/internal/routing"
)

// routeCandidates returns the networks the split-routing screen offers.
func routeCandidates(ip string) []routing.Candidate { return routing.Candidates(ip) }

// pingResult is what one ping is reported as.
type pingResult struct {
	Online bool
	RTT    int
	Err    error
}

// pingOne probes one address and returns its result in milliseconds.
func pingOne(ip string, timeoutMs float64) pingResult {
	d := time.Duration(timeoutMs) * time.Millisecond
	if d <= 0 {
		d = time.Second
	}
	r := pingfan.Probe(ip, d)
	return pingResult{Online: r.Online, RTT: int(r.RTT / time.Millisecond), Err: r.Err}
}

// RouteView draws the split-routing screen with unified Fluent cards,
// structured filterable route table with one-click delete, footerBar & statusBar.
func (a *App) RouteView(c *ui.Context) {
	useTheme(c)
	ad := a.currentAdapter()
	adapterHeaderRight := func() {
		if ad.Name == "" {
			return
		}
		if ad.Status() == "已连接" {
			pill(c, "已连接", colSuccess, colSuccessBg, colSuccessBorder)
		} else {
			pill(c, ad.Status(), colWarn, colWarnBg, colWarnBorder)
		}
		pill(c, fmt.Sprintf("索引 %d", ad.Index), colTextMuted, colSurfaceSubtle, colBorderSoft)
	}
	ui.Column(c).Fill().Background(colBackground).FontWeight(weightRegular).Children(func() {
		ui.Column(c).Grow(1).Padding(windowPad).Gap(rowGap).Children(func() {
			ui.Tabs(c, &a.Route.Tab, "分流路由配置", "IPv4 路由表管理").Height(34).FontSize(bodySize)
			if a.Route.Tab == 0 {
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Gap(colGap).Children(func() {
						panelHeader(c, "当前网卡", 0, adapterHeaderRight, func() {
							ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
								ui.Text(c, emptyDash(ad.Name)).Grow(1).FontSize(bodySize).TextColor(colText).SingleLine()
								button(c, "刷新网卡信息", 124, func() {
									a.refreshData()
									if cur := a.currentAdapter(); cur.Name != "" {
										a.Route.Gateway = routing.GatewayFor(cur.Gateways, a.savedGatewayForLocation(), a.Route.Net)
									}
									a.computeRoute()
								}).Disabled(a.busyNow())
								button(c, "查看路由表", 108, a.loadRouteTable).Disabled(a.busyNow())
							})
							ui.Row(c).Height(22).Gap(colGap).AlignItems(ui.Center).Children(func() {
								hint(c, fmt.Sprintf("IPv4：%s    掩码：%s    默认网关：%s",
									emptyDash(ad.CurrentIP()), emptyDash(firstMask(ad.Masks)), emptyDash(firstOr(ad.Gateways))))
							})
							switch {
							case ad.Name == "":
								note(c, "没有可用的网卡，请先刷新网卡信息。")
							case ad.CurrentIP() == "":
								note(c, "当前网卡没有 IPv4 地址，请先应用一个静态 IP。")
							case strings.HasPrefix(ad.CurrentIP(), "169.254."):
								note(c, "当前网卡是 169.254 自动地址，请先应用一个静态 IP。")
							}
						})
						panel(c, "分流设置", 0, func() {
							cands := routeCandidates(ad.CurrentIP())
							candLabels := make([]string, 0, len(cands)+1)
							candLabels = append(candLabels, "（手动填写）")
							for _, cand := range cands {
								candLabels = append(candLabels, cand.Label)
							}
							if a.Route.NetChoice == "" {
								a.Route.NetChoice = "（手动填写）"
							}
							formRow(c, "候选网段:", func() {
								sel := ui.Select(c, &a.Route.NetChoice, candLabels).
									Grow(1).Height(rowH - 2).FontSize(bodySize)
								if sel.Changed() {
									for _, cand := range cands {
										if cand.Label != a.Route.NetChoice {
											continue
										}
										a.Route.Net = cand.Net
										a.Route.Mask = cand.Mask
										a.Route.MaskChoice = maskLabel(cand.Mask)
										if a.Route.Gateway == "" || !a.Route.KeepDefault {
											a.Route.Gateway = routing.GatewayFor(ad.Gateways, a.savedGatewayForLocation(), cand.Net)
										}
										a.computeRoute()
									}
								}
							})
							formRow(c, "目标网段:", func() {
								if monoField(c, &a.Route.Net, 220).Placeholder("192.168.0.0").Changed() {
									a.Route.NetChoice = "（手动填写）"
									a.computeRoute()
								}
								hint(c, "需要走内网网段的地址范围")
							})
							masks := routing.Masks()
							maskLabels := make([]string, 0, len(masks))
							for _, m := range masks {
								maskLabels = append(maskLabels, m.Label)
							}
							if a.Route.MaskChoice == "" && a.Route.Mask != "" {
								a.Route.MaskChoice = maskLabel(a.Route.Mask)
							}
							formRow(c, "子网掩码:", func() {
								sel := ui.Select(c, &a.Route.MaskChoice, maskLabels).Width(250).Height(rowH - 2).FontSize(bodySize)
								if sel.Changed() {
									for _, m := range masks {
										if m.Label == a.Route.MaskChoice {
											a.Route.Mask = m.Mask
											a.computeRoute()
										}
									}
								}
								ui.Text(c, a.Route.Mask).FontSize(bodySize).TextColor(colTextMuted).SingleLine()
							})
							formRow(c, "下一跳网关:", func() {
								if monoField(c, &a.Route.Gateway, 220).Placeholder("192.168.1.1").Changed() {
									a.computeRoute()
								}
							})
							ui.Column(c).Gap(unit).Children(func() {
								if ui.Checkbox(c, &a.Route.KeepDefault, "保留默认路由（不执行 route delete 0.0.0.0）").FontSize(bodySize).Changed() {
									a.computeRoute()
								}
								if ui.Checkbox(c, &a.Route.Persist, "重启后保留这条静态路由（route add … -p）").FontSize(bodySize).Changed() {
									a.computeRoute()
								}
							})
							hint(c, "目标网段与网卡同网段时视为直连，只删默认路由，不再添加静态路由。")
							note(c, a.Route.Err)
						})
						panelHeader(c, "将执行的路由命令", 0, func() {
							if len(a.Route.Preview) > 0 {
								quickChip(c, "复制命令", false, func() {
									c.WriteClipboard(strings.Join(a.Route.Preview, "\n"))
									a.setStatus("✓ 已复制路由命令到剪贴板")
								})
							}
						}, func() {
							ui.Column(c).Padding(10).Gap(4).Background(colCodeBg).Radius(ctrlRadius).Children(func() {
								if len(a.Route.Preview) == 0 {
									ui.Text(c, "# 填好上方字段后将自动预览待执行的 route 命令").Font(monoFont).FontSize(hintSize).TextColor(colCodeMuted)
								} else {
									for _, line := range a.Route.Preview {
										ui.Text(c, "> "+line).Font(monoFont).FontSize(hintSize).TextColor(colCodeText).SingleLine()
									}
								}
							})
						})
					})
				})
			} else {
				a.routeTablePanel(c)
			}
		})
		if a.Route.Tab == 0 {
			footerBar(c, func() {
				button(c, "刷新预览", 108, func() {
					a.computeRoute()
					a.setStatus("预览已刷新")
				})
				ui.Spacer(c).Grow(1)
				primaryButton(c, "执行", 104, a.applyRoute).Disabled(a.busyNow())
				button(c, "关闭", 84, func() { a.closeWindow("route") })
			})
		} else {
			footerBar(c, func() {
				button(c, "刷新路由表", 112, a.loadRouteTable).Disabled(a.busyNow())
				ui.Spacer(c).Grow(1)
				button(c, "关闭", 84, func() { a.closeWindow("route") })
			})
		}
		statusBar(c, a.statusText(), a.Version(), a.busyNow())
	})
}

func (a *App) routeTablePanel(c *ui.Context) {
	filtered := a.filteredRoutes()
	modeLabel := "原始输出"
	if a.Route.ShowRawOutput {
		modeLabel = "可视化列表"
	}
	headerRight := func() {
		pill(c, fmt.Sprintf("%d 条", len(filtered)), colTextMuted, colSurfaceSubtle, colBorderSoft)
		quickChip(c, modeLabel, false, func() {
			a.Route.ShowRawOutput = !a.Route.ShowRawOutput
		})
		if a.Route.Output != "" {
			quickChip(c, "复制原文", false, func() {
				c.WriteClipboard(a.Route.Output)
				a.setStatus("✓ 已复制路由表原文")
			})
		}
	}
	panelHeader(c, "IPv4 路由表管理（支持一键删除静态/永久路由）", 0, headerRight, func() {
		if a.Route.ShowRawOutput {
			ui.Scroll(c).Grow(1).Background(colCodeBg).Radius(ctrlRadius).Padding(10).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					for _, line := range strings.Split(a.Route.Output, "\n") {
						ui.Text(c, line).Font(monoFont).FontSize(hintSize).TextColor(colCodeText).SingleLine()
					}
				})
			})
			return
		}
		ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
			searchField(c, &a.Route.Filter, "按目标网段、掩码、网关或接口筛选").Grow(1)
			ui.Checkbox(c, &a.Route.OnlyPersistent, "仅看永久路由").FontSize(bodySize)
		})
		ui.Row(c).Height(24).PaddingX(6).Gap(rowGap).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "目标网段").Width(140).FontSize(hintSize).FontWeight(weightHeading).TextColor(colLabel)
			ui.Text(c, "子网掩码").Width(140).FontSize(hintSize).FontWeight(weightHeading).TextColor(colLabel)
			ui.Text(c, "网关").Width(140).FontSize(hintSize).FontWeight(weightHeading).TextColor(colLabel)
			ui.Text(c, "类型 / 跃点").Grow(1).FontSize(hintSize).FontWeight(weightHeading).TextColor(colLabel)
			ui.Box(c).Width(60)
		})
		ui.Scroll(c).Grow(1).Children(func() {
			if len(filtered) == 0 {
				hint(c, "尚未加载路由表或没有匹配条目，请点击左下角「刷新路由表」。")
				return
			}
			ui.Column(c).Gap(4).Children(func() {
				for i, entry := range filtered {
					entry := entry
					key := fmt.Sprintf("rt-%d-%s-%s-%v", i, entry.Dest, entry.Gateway, entry.Persistent)
					ui.Row(c.Key(key)).Height(rowH).PaddingX(6).Gap(rowGap).AlignItems(ui.Center).
						Background(colSurfaceSubtle).Border(1, colBorderSoft).Radius(ctrlRadius).Children(func() {
						ui.Text(c, entry.Dest).Width(140).FontSize(hintSize).TextColor(colText).SingleLine()
						ui.Text(c, entry.Mask).Width(140).FontSize(hintSize).TextColor(colText).SingleLine()
						ui.Text(c, entry.Gateway).Width(140).FontSize(hintSize).TextColor(colLabel).SingleLine()
						if entry.Persistent {
							pill(c, "永久 · "+entry.Metric, colInfo, colInfoBg, colInfoBorder)
						} else {
							ui.Text(c, "活动 · "+entry.Metric).FontSize(hintSize).TextColor(colTextMuted).SingleLine()
						}
						ui.Spacer(c).Grow(1)
						ui.Button(c, "删除").Size(60, rowH-6).FontSize(hintSize).TextColor(colDanger).
							Disabled(a.busyNow()).OnClick(func() {
							a.deleteRouteEntry(entry)
						})
					})
				}
			})
		})
	}).Grow(1)
}

func (a *App) filteredRoutes() []routing.RouteEntry {
	q := strings.ToLower(strings.TrimSpace(a.Route.Filter))
	var out []routing.RouteEntry
	for _, e := range a.Route.Entries {
		if a.Route.OnlyPersistent && !e.Persistent {
			continue
		}
		if q != "" {
			if !strings.Contains(strings.ToLower(e.Dest), q) &&
				!strings.Contains(strings.ToLower(e.Mask), q) &&
				!strings.Contains(strings.ToLower(e.Gateway), q) &&
				!strings.Contains(strings.ToLower(e.Interface), q) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// savedGatewayForLocation returns the gateway the saved profiles know for the
// location being edited, as the batch script does.
func (a *App) savedGatewayForLocation() string {
	site := strings.TrimSpace(a.Pre.Location)
	if site == "" {
		return ""
	}
	var gw string
	for _, p := range a.profiles {
		if p.Location == site && p.PrimaryGateway() != "" {
			gw = p.PrimaryGateway()
		}
	}
	return gw
}

func maskLabel(mask string) string {
	for _, m := range routing.Masks() {
		if m.Mask == mask {
			return m.Label
		}
	}
	return ""
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func firstOr(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[0]
}
