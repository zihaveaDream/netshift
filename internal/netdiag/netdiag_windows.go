//go:build windows

// Package netdiag provides IP conflict detection (SendARP + ICMP) and
// concurrent connectivity diagnostics (gateway, DNS, internet and TCP ports).
package netdiag

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"netshift/internal/pingfan"
)

var (
	iphlpapi    = syscall.NewLazyDLL("iphlpapi.dll")
	procSendARP = iphlpapi.NewProc("SendARP")
)

// ConflictResult reports whether an IPv4 address appears occupied on the LAN.
type ConflictResult struct {
	IP       string
	Occupied bool
	MAC      string // AA-BB-CC-DD-EE-FF when discovered via ARP
	Method   string // "ARP" or "ICMP"
	RTT      time.Duration
	Err      error
}

// CheckConflict probes whether targetIP is currently in use on the network.
// localMACs (normalized 12-hex uppercase strings) are ignored so the machine's
// own current IP does not trigger a false positive.
func CheckConflict(targetIP string, localMACs map[string]bool, timeout time.Duration) ConflictResult {
	ipStr := strings.TrimSpace(targetIP)
	res := ConflictResult{IP: ipStr}
	ip4 := net.ParseIP(ipStr).To4()
	if ip4 == nil {
		res.Err = fmt.Errorf("不是合法的 IPv4 地址：%s", targetIP)
		return res
	}
	if timeout <= 0 {
		timeout = 1200 * time.Millisecond
	}

	// 1. Fast L2 check via SendARP (works even if remote host blocks ICMP).
	dest := binary.LittleEndian.Uint32(ip4)
	var macBuf [8]byte
	macLen := uint32(6)
	ret, _, _ := procSendARP.Call(
		uintptr(dest),
		0,
		uintptr(unsafe.Pointer(&macBuf[0])),
		uintptr(unsafe.Pointer(&macLen)),
	)
	if ret == 0 && macLen == 6 {
		norm := fmt.Sprintf("%02X%02X%02X%02X%02X%02X",
			macBuf[0], macBuf[1], macBuf[2], macBuf[3], macBuf[4], macBuf[5])
		formatted := fmt.Sprintf("%02X-%02X-%02X-%02X-%02X-%02X",
			macBuf[0], macBuf[1], macBuf[2], macBuf[3], macBuf[4], macBuf[5])
		if norm != "000000000000" && !localMACs[norm] {
			res.Occupied = true
			res.MAC = formatted
			res.Method = "ARP"
			return res
		}
		if localMACs[norm] {
			// Belongs to this machine itself; not a conflict.
			return res
		}
	}

	// 2. Fallback ICMP probe (for routed targets outside local L2 broadcast domain).
	p := pingfan.Probe(ipStr, timeout)
	if p.Online {
		res.Occupied = true
		res.Method = "ICMP"
		res.RTT = p.RTT
	}
	return res
}

// CheckItem is one diagnostic probe result.
type CheckItem struct {
	Name   string
	Target string
	OK     bool
	Detail string
	RTT    time.Duration
}

// Report aggregates the concurrent connectivity checks.
type Report struct {
	At    time.Time
	Items []CheckItem
}

// RunBasic runs gateway ping, DNS resolution and internet reachability checks
// concurrently so the whole report completes in ~1 second.
func RunBasic(ctx context.Context, gateway, dnsHost, internetHost string) Report {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(dnsHost) == "" {
		dnsHost = "www.baidu.com"
	}
	if strings.TrimSpace(internetHost) == "" {
		internetHost = "223.5.5.5"
	}

	items := make([]CheckItem, 3)
	var wg sync.WaitGroup

	// 1. Default gateway ping.
	wg.Add(1)
	go func() {
		defer wg.Done()
		gw := strings.TrimSpace(gateway)
		if gw == "" {
			items[0] = CheckItem{Name: "默认网关", Target: "未配置", OK: false, Detail: "当前网卡无默认网关"}
			return
		}
		r := pingfan.Probe(gw, 1500*time.Millisecond)
		if r.Online {
			items[0] = CheckItem{
				Name:   "默认网关",
				Target: gw,
				OK:     true,
				RTT:    r.RTT,
				Detail: fmt.Sprintf("连通正常（%d ms）", r.RTT.Milliseconds()),
			}
		} else {
			items[0] = CheckItem{Name: "默认网关", Target: gw, OK: false, Detail: "无应答或不可达"}
		}
	}()

	// 2. DNS resolution.
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		dctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
		defer cancel()
		addrs, err := net.DefaultResolver.LookupIP(dctx, "ip4", dnsHost)
		elapsed := time.Since(start)
		if err != nil || len(addrs) == 0 {
			detail := "解析失败"
			if err != nil {
				detail = err.Error()
			}
			items[1] = CheckItem{Name: "DNS 解析", Target: dnsHost, OK: false, RTT: elapsed, Detail: detail}
			return
		}
		items[1] = CheckItem{
			Name:   "DNS 解析",
			Target: dnsHost,
			OK:     true,
			RTT:    elapsed,
			Detail: fmt.Sprintf("→ %s（%d ms）", addrs[0].String(), elapsed.Milliseconds()),
		}
	}()

	// 3. Internet reachability (ICMP, falling back to TCP:53 in ICMP-filtered nets).
	wg.Add(1)
	go func() {
		defer wg.Done()
		r := pingfan.Probe(internetHost, 1500*time.Millisecond)
		if r.Online {
			items[2] = CheckItem{
				Name:   "外网连通",
				Target: internetHost,
				OK:     true,
				RTT:    r.RTT,
				Detail: fmt.Sprintf("ICMP 可达（%d ms）", r.RTT.Milliseconds()),
			}
			return
		}
		tcp := ProbeTCP(ctx, internetHost, 53, 1800*time.Millisecond)
		if tcp.OK {
			items[2] = CheckItem{
				Name:   "外网连通",
				Target: internetHost + ":53",
				OK:     true,
				RTT:    tcp.RTT,
				Detail: fmt.Sprintf("TCP 可达（%d ms）", tcp.RTT.Milliseconds()),
			}
			return
		}
		items[2] = CheckItem{Name: "外网连通", Target: internetHost, OK: false, Detail: "无法访问公网"}
	}()

	wg.Wait()
	return Report{At: time.Now(), Items: items}
}

// ProbeTCP tests whether host:port accepts a TCP connection within timeout.
func ProbeTCP(ctx context.Context, host string, port int, timeout time.Duration) CheckItem {
	host = strings.TrimSpace(host)
	target := fmt.Sprintf("%s:%d", host, port)
	if host == "" {
		return CheckItem{Name: "TCP 端口", Target: target, OK: false, Detail: "目标主机不能为空"}
	}
	if port < 1 || port > 65535 {
		return CheckItem{Name: "TCP 端口", Target: target, OK: false, Detail: "端口应在 1..65535 之间"}
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if ctx == nil {
		ctx = context.Background()
	}
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	elapsed := time.Since(start)
	if err != nil {
		return CheckItem{
			Name:   "TCP 端口",
			Target: target,
			OK:     false,
			RTT:    elapsed,
			Detail: fmt.Sprintf("连接失败（%d ms）", elapsed.Milliseconds()),
		}
	}
	_ = conn.Close()
	return CheckItem{
		Name:   "TCP 端口",
		Target: target,
		OK:     true,
		RTT:    elapsed,
		Detail: fmt.Sprintf("端口开放（%d ms）", elapsed.Milliseconds()),
	}
}
