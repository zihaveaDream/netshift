package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"netshift/internal/model"
)

// filteredProfiles returns the profiles the presets list shows, matching name,
// location, IP, gateway or computer name case-insensitively.
func (a *App) filteredProfiles() []model.Profile {
	q := strings.ToLower(strings.TrimSpace(a.Pre.Query))
	if q == "" {
		return a.profiles
	}
	var out []model.Profile
	for _, p := range a.profiles {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Location), q) ||
			strings.Contains(strings.ToLower(p.PrimaryIP()), q) ||
			strings.Contains(strings.ToLower(p.PrimaryGateway()), q) ||
			strings.Contains(strings.ToLower(p.Computer), q) {
			out = append(out, p)
		}
	}
	return out
}

// loadIntoEditor puts a profile into the presets editor.
func (a *App) loadIntoEditor(p model.Profile) {
	a.Pre.Name = p.Name
	a.Pre.EditingOld = p.Name
	a.Pre.Location = p.Location
	a.Pre.Edit = Form{}
	a.Pre.Edit.LoadFrom(p)
}

// PresetsView draws the presets window: saved profiles on the left, editor on
// the right, dedicated action bar and status bar at the bottom.
func (a *App) PresetsView(c *ui.Context) {
	useTheme(c)
	list := a.filteredProfiles()
	ui.Column(c).Fill().Background(colBackground).FontWeight(weightRegular).Children(func() {
		ui.Row(c).Grow(1).Padding(windowPad).Gap(colGap).AlignItems(ui.Stretch).Children(func() {
			a.presetsListPanel(c, list)
			a.presetsEditorPanel(c)
		})
		footerBar(c, func() {
			button(c, "重新加载方案", 122, func() {
				if loaded, err := a.store.Load(); err == nil {
					a.profiles = loaded
					a.setStatus(fmt.Sprintf("✓ 已重新加载 %d 个方案", len(loaded)))
				} else {
					a.setStatus("✗ " + err.Error())
				}
			})
			button(c, "载入到主界面", 122, func() {
				a.Form = a.Pre.Edit
				a.loadAdvancedFromForm()
				a.setStatus("✓ 已载入到主界面，确认后点「应用配置」")
			})
			dangerButton(c, "删除当前方案", 122, a.deleteCurrentProfile)
			ui.Spacer(c).Grow(1)
			button(c, "保存方案修改", 122, func() { a.saveEditor(false) })
			button(c, "另存为新方案", 122, func() { a.saveEditor(true) })
			primaryButton(c, "应用以上配置", 126, a.applyEditor).Disabled(a.busyNow())
			button(c, "关闭", 70, func() { a.closeWindow("presets") })
		})
		statusBar(c, a.statusText(), a.Version(), a.busyNow())
	})
}

func (a *App) presetsListPanel(c *ui.Context, list []model.Profile) {
	headerRight := func() {
		pill(c, fmt.Sprintf("%d", len(list)), colTextMuted, colSurfaceSubtle, colBorderSoft)
	}
	panelHeader(c, "方案列表", 276, headerRight, func() {
		searchField(c, &a.Pre.Query, "搜索方案、地点或 IP")
		ui.Scroll(c).Grow(1).Children(func() {
			if len(list) == 0 {
				ui.Column(c).PaddingY(16).Gap(4).Children(func() {
					if strings.TrimSpace(a.Pre.Query) != "" {
						hint(c, "没有匹配当前关键词的方案。")
					} else {
						hint(c, "还没有保存的方案。填好右侧配置后点「另存为新方案」。")
					}
				})
				return
			}
			ui.Column(c).Gap(3).Children(func() {
				for i, p := range list {
					i, p := i, p
					selected := a.Pre.EditingOld == p.Name || (a.Pre.EditingOld == "" && a.Pre.Name == p.Name)
					row := ui.Row(c.Key("preset-"+p.Name)).Height(44).PaddingX(8).Gap(8).
						AlignItems(ui.Center).Radius(ctrlRadius)
					if selected {
						row = row.Background(colSurfacePressed).Border(1, colInfoBorder)
					} else {
						row = row.Background(colSurfaceSubtle).Border(1, colBorderSoft)
					}
					row.OnClick(func() {
						a.Pre.Selected = i
						a.loadIntoEditor(p)
						a.setStatus("已选择方案 " + p.Name)
					}).Children(func() {
						if selected {
							ui.Box(c).Size(3, 24).Background(colAccent).Radius(2)
						}
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Text(c, p.Name).FontSize(bodySize).FontWeight(weightHeading).TextColor(colText).SingleLine()
							sub := p.PrimaryIP()
							if p.DHCP {
								sub = "自动获取 (DHCP)"
							}
							if p.Location != "" {
								sub = p.Location + " · " + sub
							}
							ui.Text(c, sub).FontSize(hintSize).TextColor(colTextMuted).SingleLine()
						})
						if p.Usage > 0 {
							ui.Text(c, fmt.Sprintf("%d次", p.Usage)).FontSize(hintSize).TextColor(colTextMuted).SingleLine()
						}
					})
				}
			})
		})
	})
}

func (a *App) presetsEditorPanel(c *ui.Context) {
	panel(c, "待应用的配置信息", 0, func() {
		formRow(c, "方案名:", func() {
			textField(c, &a.Pre.Name, 0).Grow(1).Placeholder("例如 办公室")
		})
		formRow(c, "项目地点:", func() {
			textField(c, &a.Pre.Location, 0).Grow(1).Placeholder("例如 北京-XX")
			button(c, "继承同地点配置", 136, a.inheritFromLocation)
		})
		ui.Divider(c)
		ui.RadioGroup(c, func() {
			ui.Radio(c, &a.Pre.Edit.Mode, 0, "自动获取地址").FontSize(bodySize)
			ui.Radio(c, &a.Pre.Edit.Mode, 1, "使用以下地址").FontSize(bodySize)
		}).Row().Gap(colGap)
		auto := a.Pre.Edit.Mode == 0
		editMono := func(label string, value *string, disabled bool, ph string) {
			formRow(c, label, func() {
				monoField(c, value, 0).Grow(1).Disabled(disabled).Placeholder(ph)
			})
		}
		editText := func(label string, value *string, ph string) {
			formRow(c, label, func() {
				textField(c, value, 0).Grow(1).Placeholder(ph)
			})
		}
		editMono("IP 地址:", &a.Pre.Edit.IP, auto, "192.168.1.10")
		editMono("子网掩码:", &a.Pre.Edit.Mask, auto, "255.255.255.0")
		editMono("默认网关:", &a.Pre.Edit.Gateway, auto, "192.168.1.1")
		editMono("首选 DNS:", &a.Pre.Edit.DNS1, false, "192.168.1.1")
		editMono("备用 DNS:", &a.Pre.Edit.DNS2, false, "8.8.8.8")
		ui.Divider(c)
		editText("计算机名:", &a.Pre.Edit.Computer, "可选，留空不修改")
		editText("工作组:", &a.Pre.Edit.Workgroup, "可选，留空不修改")
		editMono("MAC 地址:", &a.Pre.Edit.MAC, false, "可选，AA-BB-CC-DD-EE-FF")
	}).Grow(1)
}

// saveCurrent opens the presets window with the main window's configuration
// loaded as a new profile.
func (a *App) saveCurrent() {
	a.Pre.Edit = a.Form
	a.Pre.Name = a.profileName()
	a.Pre.EditingOld = ""
	a.Pre.Selected = -1
	a.openPresets()
	a.setStatus("确认方案名后点「另存为新方案」")
}

// profileFromEditor builds a profile from the editor's fields.
func (a *App) profileFromEditor() model.Profile {
	return a.Pre.Edit.Profile(strings.TrimSpace(a.Pre.Name), strings.TrimSpace(a.Pre.Location), a.adapter)
}

// saveEditor persists the preset editor's fields cleanly to ip.dat.
func (a *App) saveEditor(asNew bool) {
	p := a.profileFromEditor()
	oldName := ""
	if !asNew {
		oldName = a.Pre.EditingOld
	}
	if err := a.saveProfileModel(p, oldName, asNew); err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	a.Pre.Name = p.Name
	a.Pre.EditingOld = p.Name
	if asNew {
		a.setStatus("✓ 已另存为方案 " + p.Name)
	} else {
		a.setStatus("✓ 已保存方案修改 " + p.Name)
	}
}

// inheritFromLocation fills the editor's mask/gateway/DNS from the most used
// profile of the same location, preserving an IP the user already typed.
func (a *App) inheritFromLocation() {
	site := strings.TrimSpace(a.Pre.Location)
	if site == "" {
		a.setStatus("✗ 请先填写项目地点")
		return
	}
	var pick *model.Profile
	for i := range a.profiles {
		p := &a.profiles[i]
		if p.Location != site || p.Name == a.Pre.Name {
			continue
		}
		if pick == nil || p.Usage > pick.Usage {
			pick = p
		}
	}
	if pick == nil {
		a.setStatus("✗ 地点「" + site + "」还没有可继承的方案")
		return
	}
	if strings.TrimSpace(a.Pre.Edit.IP) == "" {
		a.Pre.Edit.IP = pick.PrimaryIP()
	}
	a.Pre.Edit.Mask = pick.PrimaryMask()
	a.Pre.Edit.Gateway = pick.PrimaryGateway()
	a.Pre.Edit.DNS1 = pick.PrimaryDNS()
	a.Pre.Edit.DNS2 = pick.SecondaryDNS()
	a.Pre.Edit.Mode = pickMode(pick)
	a.setStatus("✓ 已从方案「" + pick.Name + "」继承掩码/网关/DNS，可修改 IP")
}

func pickMode(p *model.Profile) int {
	if p.DHCP {
		return 0
	}
	return 1
}

// deleteCurrentProfile removes the profile the editor holds.
func (a *App) deleteCurrentProfile() {
	name := strings.TrimSpace(a.Pre.Name)
	if name == "" {
		a.setStatus("✗ 请先选择要删除的方案")
		return
	}
	if err := a.deleteProfile(name); err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	a.setStatus("✓ 已删除方案 " + name)
	a.Pre.Name, a.Pre.EditingOld, a.Pre.Location = "", "", ""
	a.Pre.Edit = Form{}
}

// applyEditor applies the editor's configuration to the selected adapter.
func (a *App) applyEditor() {
	p := a.profileFromEditor()
	if err := p.Validate(); err != nil {
		a.setStatus("✗ " + err.Error())
		return
	}
	a.applyProfile(p)
}
