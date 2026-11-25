package ui

import (
	"fmt"

	"github.com/egoist/mygo"

	"netshift/internal/assets"
)

// SetupTray creates the Windows notification-area tray icon with quick preset
// switching, DHCP restore and window show/quit actions.
func (a *App) SetupTray() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "com.oayy.netshift.tray",
		Icon:    assets.IconPNG,
		ToolTip: "NetShift · 网络配置工具箱",
		Menu:    a.buildTrayMenu(),
	})
	if err != nil {
		return
	}
	tray.OnClick(func() {
		a.showMainWindow()
	})
	a.tray = tray
}

// RefreshTrayMenu rebuilds the tray context menu whenever saved profiles or
// the active adapter change.
func (a *App) RefreshTrayMenu() {
	if a.tray == nil {
		return
	}
	a.tray.SetMenu(a.buildTrayMenu())
}

func (a *App) showMainWindow() {
	if a.main == nil || a.main.IsDestroyed() {
		return
	}
	if a.main.IsMinimized() {
		a.main.Restore()
	}
	a.main.Show()
	a.main.Focus()
}

func (a *App) buildTrayMenu() *mygo.Menu {
	adLabel := "当前网卡：无"
	if ad, ok := a.currentAdapterOK(); ok {
		adLabel = "网卡：" + ad.Label()
	}
	items := []*mygo.MenuItem{
		{
			Label: "显示主窗口",
			Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.showMainWindow() },
		},
		{
			Label:    adLabel,
			Disabled: true,
		},
		mygo.Separator(),
	}

	if len(a.profiles) == 0 {
		items = append(items, &mygo.MenuItem{
			Label:    "（暂无预设方案）",
			Disabled: true,
		})
	} else {
		limit := len(a.profiles)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			p := a.profiles[i].Clone()
			sub := "DHCP"
			if !p.DHCP && p.PrimaryIP() != "" {
				sub = p.PrimaryIP()
			}
			items = append(items, &mygo.MenuItem{
				Label: fmt.Sprintf("切换方案：%s (%s)", p.Name, sub),
				Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
					a.applyProfile(p)
				},
			})
		}
	}

	items = append(items,
		mygo.Separator(),
		&mygo.MenuItem{
			Label: "恢复自动获取 (DHCP)",
			Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.applyDHCP() },
		},
		&mygo.MenuItem{
			Label: "网络连通性诊断",
			Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				a.showMainWindow()
				a.openDiag()
			},
		},
		&mygo.MenuItem{
			Label: "代理与 Hosts 管理",
			Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				a.showMainWindow()
				a.openSysNet()
			},
		},
		&mygo.MenuItem{
			Label: "刷新网卡数据",
			Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.refreshData() },
		},
		mygo.Separator(),
		&mygo.MenuItem{
			Label: "退出程序",
			Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				if a.tray != nil {
					a.tray.Destroy()
				}
				mygo.App.Quit()
			},
		},
	)
	return mygo.NewMenu(items)
}
