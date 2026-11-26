// Command netshift is a Windows native network configuration and operations
// toolbox written in Go with mygo's native UI. Inspired by kn007's IP 地址修改器
// 5.0 and mulNetworkTest.bat, it manages adapters, IPv4/DNS, computer name,
// workgroup, MAC address, presets, split routing, diagnostics, proxy and hosts.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"

	"netshift/internal/assets"
	"netshift/internal/netcfg"
	appui "netshift/internal/ui"
)

// version is shown in the status bar and the about window.
const version = "0.1.1"

const (
	appTitle  = "NetShift · 网络配置工具箱"
	mutexName = `Local\NetShiftNativeUI`
)

func main() {
	// Text stays crisp on a scaled display: ask for real pixels first.
	netcfg.EnableDPIAwareness()

	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	dir := filepath.Dir(exe)

	mutexHandle, running, err := alreadyRunning(mutexName)
	if err != nil {
		log.Printf("单实例检测失败，继续启动：%v", err)
	}
	if mutexHandle != 0 {
		defer windows.CloseHandle(mutexHandle)
	}
	if running {
		// Exit silently when another instance is already running.
		return
	}

	// Changing network settings needs an administrator token; the original
	// tool asks for one with a manifest, this one through the shell.
	if !netcfg.IsElevated() && os.Getenv("NETSHIFT_NO_ELEVATE") == "" {
		if err := netcfg.RelaunchAsAdmin(exe, os.Args[1:]); err != nil {
			netcfg.MessageBox(appTitle, "需要管理员权限才能修改网络配置。\n请右键程序，选择「以管理员身份运行」。\n\n"+err.Error())
		}
		return
	}

	app := appui.New(dir, version)
	wantW, wantH := app.IdealMainSize()
	width, height := windowSize(wantW, wantH)
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     appTitle,
			Width:     width,
			Height:    height,
			MinWidth:  minInt(width, 920),
			MinHeight: minInt(height, 580),
			Content:   ui.View(app.MainView),
		})
		_ = win.SetIcon(assets.IconPNG)
		app.SetMainWindow(win)
		app.SetupTray()
	})
	if err := mygo.App.Run(); err != nil {
		netcfg.MessageBox(appTitle, fmt.Sprintf("启动失败：%v", err))
		log.Fatal(err)
	}
}

// windowSize adapts the desired main window size (computed from adapter label
// length) to the screen's available work area in DIPs, so long adapter names
// display in full whenever the display has room while never overflowing.
func windowSize(wantW, wantH int) (int, int) {
	w, h, scale := netcfg.WorkAreaDIPs()
	if w <= 0 || h <= 0 {
		return wantW, wantH
	}
	width, height := wantW, wantH
	if maxW := w - 48; width > maxW {
		width = maxW
	}
	if maxH := h - 48; height > maxH {
		height = maxH
	}
	if width < 780 {
		width = 780
	}
	if height < 520 {
		height = 520
	}
	if os.Getenv("NETSHIFT_NO_ELEVATE") != "" {
		log.Printf("窗口 %dx%d DIP（理想 %dx%d）：可用工作区 %dx%d DIP（缩放 %v）", width, height, wantW, wantH, w, h, scale)
	}
	return width, height
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// alreadyRunning reports whether another copy holds the named mutex and
// returns the created handle so the caller keeps it alive until process exit.
func alreadyRunning(name string) (windows.Handle, bool, error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, false, err
	}
	h, err := windows.CreateMutex(nil, false, ptr)
	if err == windows.ERROR_ALREADY_EXISTS {
		return h, true, nil
	}
	return h, false, err
}
