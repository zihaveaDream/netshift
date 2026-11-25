package subnet

import "testing"

func TestComputePrefix(t *testing.T) {
	tests := []struct {
		ip     string
		prefix int
		want   Info
	}{
		{
			ip: "192.168.1.10", prefix: 24,
			want: Info{IP: "192.168.1.10", Prefix: 24, Mask: "255.255.255.0",
				Network: "192.168.1.0", Broadcast: "192.168.1.255",
				First: "192.168.1.1", Last: "192.168.1.254", Hosts: 254, Total: 256},
		},
		{
			ip: "10.20.30.40", prefix: 22,
			want: Info{IP: "10.20.30.40", Prefix: 22, Mask: "255.255.252.0",
				Network: "10.20.28.0", Broadcast: "10.20.31.255",
				First: "10.20.28.1", Last: "10.20.31.254", Hosts: 1022, Total: 1024},
		},
		{
			ip: "172.16.5.4", prefix: 12,
			want: Info{IP: "172.16.5.4", Prefix: 12, Mask: "255.240.0.0",
				Network: "172.16.0.0", Broadcast: "172.31.255.255",
				First: "172.16.0.1", Last: "172.31.255.254", Hosts: 1048574, Total: 1048576},
		},
		{
			ip: "192.168.1.10", prefix: 31,
			want: Info{IP: "192.168.1.10", Prefix: 31, Mask: "255.255.255.254",
				Network: "192.168.1.10", Broadcast: "192.168.1.11",
				First: "192.168.1.10", Last: "192.168.1.11", Hosts: 2, Total: 2},
		},
		{
			ip: "192.168.1.10", prefix: 32,
			want: Info{IP: "192.168.1.10", Prefix: 32, Mask: "255.255.255.255",
				Network: "192.168.1.10", Broadcast: "192.168.1.10",
				First: "192.168.1.10", Last: "192.168.1.10", Hosts: 1, Total: 1},
		},
	}
	for _, tc := range tests {
		got, err := ComputePrefix(tc.ip, tc.prefix)
		if err != nil {
			t.Fatalf("%s/%d: %v", tc.ip, tc.prefix, err)
		}
		if got != tc.want {
			t.Errorf("%s/%d:\n got %+v\nwant %+v", tc.ip, tc.prefix, got, tc.want)
		}
	}
}

func TestComputeRejectsBadInput(t *testing.T) {
	if _, err := ComputePrefix("192.168.1.999", 24); err == nil {
		t.Error("bad address accepted")
	}
	if _, err := ComputePrefix("192.168.1.1", 33); err == nil {
		t.Error("bad prefix accepted")
	}
	if _, err := Compute("192.168.1.1", "255.0.255.0"); err == nil {
		t.Error("non-contiguous mask accepted")
	}
}

func TestCovers(t *testing.T) {
	info, err := ComputePrefix("192.168.1.10", 24)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Covers("192.168.1.200") {
		t.Error("192.168.1.200 should be inside")
	}
	if info.Covers("192.168.2.10") {
		t.Error("192.168.2.10 should be outside")
	}
}
