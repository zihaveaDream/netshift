//go:build windows

package netcfg

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"netshift/internal/model"
)

// defaultCmdTimeout protects every external command from hanging indefinitely.
const defaultCmdTimeout = 30 * time.Second

// List returns the machine's IPv4 connections, up ones first, then by name.
// Loopback and adapters without a connection name are skipped.
func List() ([]Adapter, error) {
	addrs, err := adapterAddresses()
	if err != nil {
		return nil, err
	}
	var out []Adapter
	for _, a := range addrs {
		if a.Name == "" || strings.EqualFold(a.Name, "Loopback Pseudo-Interface 1") {
			continue
		}
		if a.Index <= 0 {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Up != out[j].Up {
			return out[i].Up
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Find returns the adapter with the given connection name.
func Find(name string) (Adapter, bool) {
	list, err := List()
	if err != nil {
		return Adapter{}, false
	}
	for _, a := range list {
		if a.Name == name {
			return a, true
		}
	}
	return Adapter{}, false
}

const (
	gaaSkipAnycast     = 0x02
	gaaSkipMulticast   = 0x04
	gaaIncludeGateways = 0x80
)

func adapterAddresses() ([]Adapter, error) {
	var out []Adapter
	size := uint32(32 * 1024)
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		raw := make([]byte, size)
		buf := (*windows.IpAdapterAddresses)(unsafe.Pointer(&raw[0]))
		err = windows.GetAdaptersAddresses(windows.AF_UNSPEC,
			gaaSkipAnycast|gaaSkipMulticast|gaaIncludeGateways, 0, buf, &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("GetAdaptersAddresses：%w", err)
		}
		for p := buf; p != nil; p = p.Next {
			var speedMbps uint64
			if p.TransmitLinkSpeed > 0 && p.TransmitLinkSpeed != ^uint64(0) {
				speedMbps = p.TransmitLinkSpeed / 1_000_000
			}
			a := Adapter{
				GUID:          guidOf(p),
				Name:          windows.UTF16PtrToString(p.FriendlyName),
				Description:   windows.UTF16PtrToString(p.Description),
				Index:         int(p.IfIndex),
				Up:            p.OperStatus == windows.IfOperStatusUp,
				MAC:           formatMAC(p.PhysicalAddress[:p.PhysicalAddressLength]),
				IfType:        p.IfType,
				LinkSpeedMbps: speedMbps,
			}
			for u := p.FirstUnicastAddress; u != nil; u = u.Next {
				ip := u.Address.IP()
				if ip == nil || ip.To4() == nil {
					continue
				}
				a.IPv4 = append(a.IPv4, ip.String())
				a.Masks = append(a.Masks, model.PrefixMask(int(u.OnLinkPrefixLength)))
			}
			for g := p.FirstGatewayAddress; g != nil; g = g.Next {
				if ip := g.Address.IP(); ip != nil && ip.To4() != nil {
					a.Gateways = append(a.Gateways, ip.String())
				}
			}
			for d := p.FirstDnsServerAddress; d != nil; d = d.Next {
				if ip := d.Address.IP(); ip != nil && ip.To4() != nil {
					a.DNS = append(a.DNS, ip.String())
				}
			}
			a.DHCP = dhcpEnabled(a.GUID)
			out = append(out, a)
		}
		return out, nil
	}
	if err == nil {
		err = fmt.Errorf("GetAdaptersAddresses 缓冲区不足")
	}
	return nil, err
}

// guidOf returns the adapter's registry GUID, the {XXXXXXXX-...} string
// GetAdaptersAddresses reports as its name.
func guidOf(p *windows.IpAdapterAddresses) string {
	if p.AdapterName == nil {
		return ""
	}
	return strings.Trim(windows.BytePtrToString(p.AdapterName), "{}")
}

func formatMAC(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(b) * 2)
	for _, v := range b {
		fmt.Fprintf(&sb, "%02X", v)
	}
	return sb.String()
}

// dhcpEnabled reads EnableDHCP from the interface's TCP/IP key: the adapter
// list API does not report it.
func dhcpEnabled(guid string) bool {
	if guid == "" {
		return false
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\{`+guid+`}`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	v, _, err := key.GetIntegerValue("EnableDHCP")
	return err == nil && v == 1
}

// Run executes a command line through cmd.exe with no console window and
// returns what it printed, in UTF-8, guarded by a default timeout.
func Run(line string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCmdTimeout)
	defer cancel()
	return RunContext(ctx, line)
}

// RunContext executes a command line through cmd.exe with cancellation/timeout.
func RunContext(ctx context.Context, line string) (string, error) {
	cmd := exec.CommandContext(ctx, "cmd", "/c", line)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(DecodeConsole(out))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return text, fmt.Errorf("命令执行超时（%v）", defaultCmdTimeout)
		}
		var runErr error = err
		if text != "" {
			runErr = fmt.Errorf("%v：%s", err, text)
		}
		return text, checkNetshResult(line, text, runErr)
	}
	if err := checkNetshResult(line, text, nil); err != nil {
		return text, err
	}
	return text, nil
}

// OpenNetworkConnections launches ncpa.cpl without blocking the caller.
func OpenNetworkConnections() error {
	cmd := exec.Command("control.exe", "ncpa.cpl")
	return cmd.Start()
}

// Apply runs the commands of r in order and stops at the first failure,
// returning the commands it ran.
func Apply(r Request) ([]string, error) { return runAll(r.Commands) }

// ApplyAddresses runs only the address commands of r.
func ApplyAddresses(r Request) ([]string, error) { return runAll(r.AddressCommands) }

// ApplyDNS sets only the DNS servers of an adapter.
func ApplyDNS(adapter string, servers []string) ([]string, error) {
	return runAll(func() ([]string, error) { return SetDNSCommands(adapter, servers) })
}

// ApplyGateway sets only the default gateway of an adapter.
func ApplyGateway(adapter, gateway string, metric int) (string, error) {
	line, err := SetGatewayCommand(adapter, gateway, metric)
	if err != nil {
		return "", err
	}
	return Run(line)
}

// ApplyGateways replaces an adapter's gateways with the given list.
func ApplyGateways(adapter string, gateways []model.Gateway) ([]string, error) {
	return runAll(func() ([]string, error) { return SetGatewaysCommands(adapter, gateways) })
}

// AddressCommands and Commands as request methods, so runAll takes either.
func (r Request) AddressCommands() ([]string, error) { return AddressCommands(r) }
func (r Request) Commands() ([]string, error)        { return Commands(r) }

func runAll(build func() ([]string, error)) ([]string, error) {
	lines, err := build()
	if err != nil {
		return nil, err
	}
	var ran []string
	for _, line := range lines {
		ran = append(ran, line)
		if _, err := Run(line); err != nil {
			return ran, fmt.Errorf("%s 失败：%w", line, err)
		}
	}
	return ran, nil
}

// SetAdapterEnabled enables or disables an adapter.
func SetAdapterEnabled(adapter string, on bool) error {
	line, err := SetEnabled(adapter, on)
	if err != nil {
		return err
	}
	_, err = Run(line)
	return err
}

// SetAdapterMetric sets the interface metric of an adapter.
func SetAdapterMetric(adapter string, metric int) error {
	line, err := SetMetric(adapter, metric)
	if err != nil {
		return err
	}
	_, err = Run(line)
	return err
}

// Release, Renew and FlushDNS are the ipconfig actions of the advanced page.
func Release(adapter string) (string, error) {
	line, err := ReleaseCommand(adapter)
	if err != nil {
		return "", err
	}
	return Run(line)
}

func Renew(adapter string) (string, error) {
	line, err := RenewCommand(adapter)
	if err != nil {
		return "", err
	}
	return Run(line)
}

func FlushDNS() (string, error) { return Run(FlushDNSCommand()) }

// AwaitAdapter polls the adapter list until pred accepts the adapter named
// name, calling onUpdate whenever its state changes. It is the Windows entry to
// awaitAdapterWith, for callers waiting out a DHCP lease or a restarted adapter.
func AwaitAdapter(ctx context.Context, name string, interval time.Duration, pred func(Adapter) bool, onUpdate func([]Adapter, Adapter)) ([]Adapter, Adapter, bool) {
	return awaitAdapterWith(ctx, name, interval, List, pred, onUpdate)
}

// IsElevated reports whether the process can change network settings.
func IsElevated() bool {
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	isUserAnAdmin := shell32.NewProc("IsUserAnAdmin")
	r, _, _ := isUserAnAdmin.Call()
	return r != 0
}

// RelaunchAsAdmin starts the executable elevated through the shell.
func RelaunchAsAdmin(exe string, args []string) error {
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	shellExecute := shell32.NewProc("ShellExecuteW")
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	var params *uint16
	if len(args) > 0 {
		params, err = windows.UTF16PtrFromString(strings.Join(args, " "))
		if err != nil {
			return err
		}
	}
	const swShowNormal = 1
	ret, _, _ := shellExecute.Call(0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)), 0, swShowNormal)
	if ret <= 32 {
		return fmt.Errorf("提权启动失败（ShellExecute 返回 %d）", ret)
	}
	return nil
}

// EnableDPIAwareness makes the process draw at the display's real resolution
// instead of being scaled up as a bitmap, which is what makes text blurry on a
// 150% or 200% display. It must run before the first window is created.
func EnableDPIAwareness() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, Windows 10 1703 and later.
	const perMonitorV2 = ^uintptr(3) // -4 as UINT_PTR
	if proc := user32.NewProc("SetProcessDpiAwarenessContext"); proc.Find() == nil {
		if ret, _, _ := proc.Call(perMonitorV2); ret != 0 {
			return
		}
	}
	if proc := user32.NewProc("SetProcessDPIAware"); proc.Find() == nil {
		proc.Call()
	}
}

// IPOf returns the parsed address of s, or nil.
func IPOf(s string) net.IP { return net.ParseIP(strings.TrimSpace(s)) }

// WorkAreaDIPs returns the desktop's work area in device independent pixels,
// and the scale it was divided by, so a window can size itself to the screen
// it opens on.
func WorkAreaDIPs() (width, height int, scale float64) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	getDpiForSystem := user32.NewProc("GetDpiForSystem")
	systemParametersInfo := user32.NewProc("SystemParametersInfoW")

	dpi := uint32(96)
	if err := getDpiForSystem.Find(); err == nil {
		if v, _, _ := getDpiForSystem.Call(); v > 0 {
			dpi = uint32(v)
		}
	}
	scale = float64(dpi) / 96

	const spiGetWorkArea = 0x0030
	var rect [4]int32 // left, top, right, bottom
	if ret, _, _ := systemParametersInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&rect[0])), 0); ret == 0 {
		return 0, 0, scale
	}
	w := int(float64(rect[2]-rect[0]) / scale)
	h := int(float64(rect[3]-rect[1]) / scale)
	return w, h, scale
}

// MessageBox shows an error dialog, for the cases where the app cannot start
// its window (no console is visible in a GUI build).
func MessageBox(title, text string) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	t, err1 := windows.UTF16PtrFromString(title)
	b, err2 := windows.UTF16PtrFromString(text)
	if err1 != nil || err2 != nil {
		return
	}
	const mbIconError = 0x10
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), mbIconError)
}
