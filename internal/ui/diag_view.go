package ui

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
)

// DiagView draws the one-click connectivity diagnostic window: concurrent
// gateway / DNS / internet reachability + custom TCP port tester.
func (a *App) DiagView(c *ui.Context) {
	useTheme(c)
	ad := a.currentAdapter()
	gw := firstOr(ad.Gateways)
	if gw == "" {
		gw = a.Form.Gateway
	}
	headerRight := func() {
		if !a.Diag.Report.At.IsZero() {
			pill(c, "更新于 "+a.Diag.Report.At.Format("15:04:05"), colTextMuted, colSurfaceSubtle, colBorderSoft)
		}
	}
	ui.Column(c).Fill().Background(colBackground).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(windowPad).Gap(colGap).Children(func() {
				panelHeader(c, "基础连通性诊断（默认网关 / DNS / 公网）", 0, headerRight, func() {
					pairRow(c, "DNS 域名:", func() {
						textField(c, &a.Diag.DNSArg, 0).Grow(1).Placeholder("www.baidu.com")
					}, "外网 IP:", 64, func() {
						textField(c, &a.Diag.InternetArg, 0).Grow(1).Placeholder("223.5.5.5")
					})
					ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
						hint(c, fmt.Sprintf("当前网卡：%s    检测网关：%s", emptyDash(ad.Name), emptyDash(gw)))
						ui.Spacer(c).Grow(1)
						primaryButton(c, "一键开始诊断", 128, a.runBasicDiag).Disabled(a.Diag.Running)
					})
					ui.Divider(c)
					if len(a.Diag.Report.Items) == 0 {
						hint(c, "点击「一键开始诊断」将并发测试网关、DNS 解析与外网连通性。")
					} else {
						ui.Column(c).Gap(6).Children(func() {
							for _, it := range a.Diag.Report.Items {
								ui.Row(c).Height(32).PaddingX(10).Gap(10).AlignItems(ui.Center).
									Background(colSurfaceSubtle).Border(1, colBorderSoft).Radius(ctrlRadius).Children(func() {
									if it.OK {
										pill(c, "正常", colSuccess, colSuccessBg, colSuccessBorder)
									} else {
										pill(c, "异常", colDanger, colDangerBg, colDangerBorder)
									}
									ui.Text(c, it.Name).Width(76).FontSize(bodySize).TextColor(colText).SingleLine()
									ui.Text(c, it.Target).Width(150).FontSize(hintSize).TextColor(colLabel).SingleLine()
									ui.Text(c, it.Detail).Grow(1).FontSize(hintSize).TextColor(colTextMuted).SingleLine()
								})
							}
						})
					}
				})

				panel(c, "TCP 端口连通探测", 0, func() {
					ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "目标主机:").Width(72).FontSize(bodySize).TextColor(colLabel).SingleLine()
						textField(c, &a.Diag.PortHost, 0).Grow(1).Placeholder("IP 或域名，如 192.168.1.1")
						ui.Text(c, "端口:").Width(42).FontSize(bodySize).TextColor(colLabel).SingleLine()
						textField(c, &a.Diag.PortNum, 76).Placeholder("80")
						button(c, "探测端口", 96, func() {
							a.runPortProbe(a.Diag.PortHost, parsePortText(a.Diag.PortNum))
						}).Disabled(a.Diag.PortRunning)
					})
					ui.Row(c).Height(24).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "常用端口:").FontSize(hintSize).TextColor(colLabel).SingleLine()
						for _, p := range []struct {
							port  int
							label string
						}{
							{22, "22 SSH"},
							{80, "80 HTTP"},
							{443, "443 HTTPS"},
							{445, "445 SMB"},
							{3306, "3306 MySQL"},
							{3389, "3389 RDP"},
						} {
							p := p
							quickChip(c, p.label, a.Diag.PortRunning, func() {
								a.Diag.PortNum = strconv.Itoa(p.port)
								a.runPortProbe(a.Diag.PortHost, p.port)
							})
						}
					})
					if len(a.Diag.PortResults) > 0 {
						ui.Divider(c)
						ui.Column(c).Gap(5).Children(func() {
							for i, it := range a.Diag.PortResults {
								ui.Row(c.Key(fmt.Sprintf("port-%d-%s", i, it.Target))).Height(28).PaddingX(8).Gap(10).
									AlignItems(ui.Center).Background(colSurfaceSubtle).Border(1, colBorderSoft).Radius(ctrlRadius).Children(func() {
									if it.OK {
										pill(c, "开放", colSuccess, colSuccessBg, colSuccessBorder)
									} else {
										pill(c, "不通", colDanger, colDangerBg, colDangerBorder)
									}
									ui.Text(c, it.Target).Width(200).FontSize(hintSize).TextColor(colText).SingleLine()
									ui.Text(c, it.Detail).Grow(1).FontSize(hintSize).TextColor(colTextMuted).SingleLine()
								})
							}
						})
					}
				})
			})
		})
		footerBar(c, func() {
			button(c, "清空探测记录", 118, func() {
				a.Diag.PortResults = nil
				a.setStatus("已清空端口探测记录")
			}).Disabled(len(a.Diag.PortResults) == 0)
			ui.Spacer(c).Grow(1)
			button(c, "关闭", 84, func() { a.closeWindow("diag") })
		})
		statusBar(c, a.statusText(), a.Version(), a.Diag.Running || a.Diag.PortRunning)
	})
}
