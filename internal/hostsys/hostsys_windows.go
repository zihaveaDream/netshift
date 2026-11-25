//go:build windows

// Package hostsys reads and changes the computer name and workgroup.
package hostsys

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"netshift/internal/netcfg"
)

// Info is what the main window shows in its computer group.
type Info struct {
	Name      string
	Workgroup string
}

// Current reads the computer name and workgroup.
func Current() (Info, error) {
	name, err := os.Hostname()
	if err != nil {
		return Info{}, err
	}
	return Info{Name: name, Workgroup: workgroup()}, nil
}

func workgroup() string {
	// Prefer the official Win32 NetGetJoinInformation API, which accurately
	// returns both workgroup and domain names on all Windows versions.
	if name := netGetJoinWorkgroup(); name != "" {
		return name
	}
	for _, path := range []string{
		`SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`,
		`SYSTEM\CurrentControlSet\Services\LanmanServer\Parameters`,
	} {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		if v, _, err := key.GetStringValue("Workgroup"); err == nil && v != "" {
			key.Close()
			return v
		}
		if v, _, err := key.GetStringValue("Domain"); err == nil && v != "" {
			key.Close()
			return v
		}
		key.Close()
	}
	return "WORKGROUP"
}

func netGetJoinWorkgroup() string {
	netapi32 := windows.NewLazySystemDLL("netapi32.dll")
	procJoinInfo := netapi32.NewProc("NetGetJoinInformation")
	procBufFree := netapi32.NewProc("NetApiBufferFree")
	if procJoinInfo.Find() != nil || procBufFree.Find() != nil {
		return ""
	}
	var namePtr *uint16
	var bufType uint32
	ret, _, _ := procJoinInfo.Call(
		0,
		uintptr(unsafe.Pointer(&namePtr)),
		uintptr(unsafe.Pointer(&bufType)),
	)
	if ret != 0 || namePtr == nil {
		return ""
	}
	defer procBufFree.Call(uintptr(unsafe.Pointer(namePtr)))
	return windows.UTF16PtrToString(namePtr)
}

// ValidateName checks a computer name against the Windows rules.
func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("计算机名不能为空")
	}
	if len(name) > 15 {
		return fmt.Errorf("计算机名不能超过 15 个字符")
	}
	for _, r := range name {
		ok := r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("计算机名只能包含字母、数字、- 和 _：%s", name)
		}
	}
	if strings.Trim(name, "0123456789") == "" {
		return fmt.Errorf("计算机名不能全为数字")
	}
	return nil
}

// ValidateWorkgroup checks a workgroup name and rejects shell metacharacters.
func ValidateWorkgroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("工作组不能为空")
	}
	if len(name) > 15 {
		return fmt.Errorf("工作组不能超过 15 个字符")
	}
	if strings.ContainsAny(name, `\/:*?"<>| '&^%$!;`+"`\r\n") {
		return fmt.Errorf(`工作组含有不支持的字符（不能含空格或特殊符号）`)
	}
	return nil
}

// Rename sets a new computer name. Windows applies it at the next restart.
// Prefers the native SetComputerNameExW API (instant, no subprocess) and falls
// back to PowerShell if needed.
func Rename(name string) (string, error) {
	name = strings.TrimSpace(name)
	if err := ValidateName(name); err != nil {
		return "", err
	}
	if err := setComputerNameWin32(name); err == nil {
		return "OK", nil
	}
	ps := fmt.Sprintf(`powershell -NoProfile -Command "Rename-Computer -NewName '%s' -Force -ErrorAction Stop; 'OK'"`, name)
	out, err := netcfg.Run(ps)
	if err != nil {
		return out, fmt.Errorf("修改计算机名失败：%w", err)
	}
	return out, nil
}

func setComputerNameWin32(name string) error {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := kernel32.NewProc("SetComputerNameExW")
	if err := proc.Find(); err != nil {
		return err
	}
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	// ComputerNamePhysicalDnsHostname = 5 updates both NetBIOS and DNS hostname.
	const computerNamePhysicalDnsHostname = 5
	r1, _, lastErr := proc.Call(computerNamePhysicalDnsHostname, uintptr(unsafe.Pointer(ptr)))
	if r1 == 0 {
		return lastErr
	}
	return nil
}

// JoinWorkgroup moves the machine into a workgroup.
func JoinWorkgroup(name string) (string, error) {
	name = strings.TrimSpace(name)
	if err := ValidateWorkgroup(name); err != nil {
		return "", err
	}
	ps := fmt.Sprintf(`powershell -NoProfile -Command "Add-Computer -WorkgroupName '%s' -Force -ErrorAction Stop; 'OK'"`, name)
	out, err := netcfg.Run(ps)
	if err != nil {
		return out, fmt.Errorf("修改工作组失败：%w", err)
	}
	return out, nil
}

var namePrefixes = []string{"PC", "WS", "DESK", "NODE", "LAB"}

// RandomName returns a random computer name in the style the original tool
// generates: a short prefix and four digits.
func RandomName() string {
	prefix := namePrefixes[rand.Intn(len(namePrefixes))]
	return fmt.Sprintf("%s-%04d", prefix, rand.Intn(10000))
}

// Restart restarts the machine, which renaming needs.
func Restart() error {
	_, err := netcfg.Run(`shutdown /r /t 5 /c "计算机名或工作组修改后需要重启"`)
	return err
}
