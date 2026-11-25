//go:build windows

package netdiag

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestCheckConflictRejectsBadIP(t *testing.T) {
	r := CheckConflict("not-an-ip", nil, 300*time.Millisecond)
	if r.Err == nil {
		t.Error("expected error for invalid IP")
	}
}

func TestProbeTCPDirectListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	res := ProbeTCP(context.Background(), "127.0.0.1", port, time.Second)
	if !res.OK {
		t.Fatalf("expected open port %d, got %+v", port, res)
	}
	bad := ProbeTCP(context.Background(), "127.0.0.1", 0, time.Second)
	if bad.OK {
		t.Error("port 0 should fail")
	}
}
