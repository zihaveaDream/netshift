//go:build windows

package pingfan

import (
	"context"
	"testing"
	"time"
)

func TestProbeLoopback(t *testing.T) {
	r := Probe("127.0.0.1", 1500*time.Millisecond)
	if r.Err != nil && r.Err.Error() != "无应答" {
		t.Fatalf("Probe failed: %v", r.Err)
	}
	if !r.Online {
		t.Errorf("127.0.0.1 reported offline (%v)", r.Err)
	}
}

func TestProbeRejectsBadAddress(t *testing.T) {
	if r := Probe("not-an-ip", time.Second); r.Err == nil {
		t.Error("a bad address should report an error")
	}
}

func TestFanCounts(t *testing.T) {
	// Fan over the loopback /24: .1 is the machine itself on Windows.
	results := Fan("127.0.0", 8, 300*time.Millisecond, nil)
	if len(results) != 254 {
		t.Fatalf("got %d results, want 254", len(results))
	}
	online, total := Summary(results)
	if total != 254 {
		t.Errorf("summary total = %d", total)
	}
	if online == 0 {
		t.Error("no address reported online, the loopback network should have one")
	}
}

func TestValidatePrefixAndCancel(t *testing.T) {
	if got, err := ValidatePrefix("192.168.1."); err != nil || got != "192.168.1" {
		t.Errorf("ValidatePrefix = %q, %v", got, err)
	}
	if _, err := ValidatePrefix("192.168.999"); err == nil {
		t.Error("invalid octet accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := FanContext(ctx, "127.0.0", 4, 200*time.Millisecond, nil)
	if len(res) != 0 {
		t.Errorf("cancelled context should yield 0 results, got %d", len(res))
	}
}
