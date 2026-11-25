//go:build windows

// Package pingfan probes the addresses of a /24 network, for the C-class
// ping screen of the toolbox. It uses IcmpSendEcho, so it needs no console
// window and no external process.
package pingfan

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Result is one probe.
type Result struct {
	IP     string
	Host   int // last octet 1..254 when probed by Fan
	Online bool
	RTT    time.Duration
	Err    error
}

type ipOptionInformation struct {
	TTL         byte
	Tos         byte
	Flags       byte
	OptionsSize byte
	OptionsData uintptr
}

type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

var (
	iphlpapi            = syscall.NewLazyDLL("iphlpapi.dll")
	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho    = iphlpapi.NewProc("IcmpSendEcho")
)

var pingPayload = []byte("netshift-ping-0123456789abcdef")

// ValidatePrefix validates a /24 prefix like "192.168.1".
func ValidatePrefix(prefix string) (string, error) {
	p := strings.TrimSpace(prefix)
	p = strings.TrimSuffix(p, ".")
	parts := strings.Split(p, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("网段前缀格式应为 a.b.c，例如 192.168.1")
	}
	var out [3]string
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return "", fmt.Errorf("网段前缀无效：%s", prefix)
		}
		out[i] = strconv.Itoa(n)
	}
	return strings.Join(out[:], "."), nil
}

// Probe sends one echo request and reports whether a reply came back.
func Probe(ip string, timeout time.Duration) Result {
	handle, _, err := procIcmpCreateFile.Call()
	if handle == 0 || handle == uintptr(syscall.InvalidHandle) {
		return Result{IP: ip, Err: fmt.Errorf("IcmpCreateFile 失败：%v", err)}
	}
	defer procIcmpCloseHandle.Call(handle)
	replyBuf := make([]byte, int(unsafe.Sizeof(icmpEchoReply{}))+len(pingPayload)+64)
	return probeWithHandle(handle, ip, 0, timeout, replyBuf)
}

func probeWithHandle(handle uintptr, ip string, host int, timeout time.Duration, replyBuf []byte) Result {
	res := Result{IP: ip, Host: host}
	addr := net.ParseIP(strings.TrimSpace(ip)).To4()
	if addr == nil {
		res.Err = fmt.Errorf("不是 IPv4 地址：%s", ip)
		return res
	}
	ms := uint32(timeout / time.Millisecond)
	if ms == 0 {
		ms = 1000
	}
	// IcmpSendEcho takes IPAddr in network byte order (in_addr laid out in memory
	// as [a0, a1, a2, a3]), which on little-endian x86/x64 is LittleEndian.Uint32.
	destAddr := binary.LittleEndian.Uint32(addr)
	n, _, _ := procIcmpSendEcho.Call(
		handle,
		uintptr(destAddr),
		uintptr(unsafe.Pointer(&pingPayload[0])),
		uintptr(len(pingPayload)),
		0,
		uintptr(unsafe.Pointer(&replyBuf[0])),
		uintptr(len(replyBuf)),
		uintptr(ms),
	)
	if n == 0 {
		res.Err = fmt.Errorf("无应答")
		return res
	}
	reply := (*icmpEchoReply)(unsafe.Pointer(&replyBuf[0]))
	res.Online = reply.Status == 0
	if res.Online {
		res.RTT = time.Duration(reply.RoundTripTime) * time.Millisecond
	} else {
		res.Err = fmt.Errorf("状态码 %d", reply.Status)
	}
	return res
}

// Fan probes prefix.1 .. prefix.254 with the given number of workers and calls
// report for every result as it arrives. Calling Fan with a nil report is safe.
func Fan(prefix string, workers int, timeout time.Duration, report func(Result)) []Result {
	return FanContext(context.Background(), prefix, workers, timeout, report)
}

// FanContext probes prefix.1 .. prefix.254 and stops early when ctx is cancelled.
// Each worker reuses a single ICMP handle and reply buffer to minimize allocations
// and kernel handle churn.
func FanContext(ctx context.Context, prefix string, workers int, timeout time.Duration, report func(Result)) []Result {
	if ctx == nil {
		ctx = context.Background()
	}
	if workers < 1 {
		workers = 16
	}
	if workers > 128 {
		workers = 128
	}
	cleanPrefix := strings.TrimSuffix(strings.TrimSpace(prefix), ".")
	jobs := make(chan int, workers)
	results := make([]Result, 0, 254)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle, _, err := procIcmpCreateFile.Call()
			hasHandle := handle != 0 && handle != uintptr(syscall.InvalidHandle)
			if hasHandle {
				defer procIcmpCloseHandle.Call(handle)
			}
			replyBuf := make([]byte, int(unsafe.Sizeof(icmpEchoReply{}))+len(pingPayload)+64)
			for n := range jobs {
				if ctx.Err() != nil {
					continue
				}
				ip := fmt.Sprintf("%s.%d", cleanPrefix, n)
				var r Result
				if !hasHandle {
					r = Result{IP: ip, Host: n, Err: fmt.Errorf("IcmpCreateFile 失败：%v", err)}
				} else {
					r = probeWithHandle(handle, ip, n, timeout, replyBuf)
				}
				if ctx.Err() != nil {
					continue
				}
				if report != nil {
					report(r)
				}
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}
		}()
	}

enqueue:
	for n := 1; n <= 254; n++ {
		select {
		case <-ctx.Done():
			break enqueue
		case jobs <- n:
		}
	}
	close(jobs)
	wg.Wait()
	return results
}

// Summary counts the online results.
func Summary(results []Result) (online, total int) {
	for _, r := range results {
		total++
		if r.Online {
			online++
		}
	}
	return online, total
}
