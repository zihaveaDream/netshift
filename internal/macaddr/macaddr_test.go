package macaddr

import (
	"strconv"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"AA-BB-CC-DD-EE-FF": "AABBCCDDEEFF",
		"aa:bb:cc:dd:ee:ff": "AABBCCDDEEFF",
		"aabbccddeeff":      "AABBCCDDEEFF",
		"AABB.CCDD.EEFF":    "AABBCCDDEEFF",
	}
	for in, want := range tests {
		got, err := Normalize(in)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%s = %s, want %s", in, got, want)
		}
	}
	bad := []string{"", "AABBCCDDEE", "AABBCCDDEEFFF", "GGHHIIJJKKLL", "000000000000", "FFFFFFFFFFFF", "01-23-45-67-89-AB"}
	for _, in := range bad {
		if _, err := Normalize(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestFormat(t *testing.T) {
	if got := Format("AABBCCDDEEFF"); got != "AA-BB-CC-DD-EE-FF" {
		t.Errorf("Format = %s", got)
	}
	if got := Format("short"); got != "short" {
		t.Errorf("Format of a short value = %s", got)
	}
}

func TestRandomIsUnicastAndLocal(t *testing.T) {
	for i := 0; i < 64; i++ {
		mac := Random()
		normalized, err := Normalize(mac)
		if err != nil {
			t.Fatalf("Random produced %q: %v", mac, err)
		}
		first, err := strconv.ParseUint(normalized[:2], 16, 8)
		if err != nil {
			t.Fatalf("%q: %v", normalized, err)
		}
		if first&0x01 != 0 {
			t.Errorf("%s is a multicast address", normalized)
		}
		if first&0x02 == 0 {
			t.Errorf("%s is not locally administered", normalized)
		}
	}
}

func TestRandomVaries(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 32; i++ {
		seen[Random()] = true
	}
	if len(seen) < 8 {
		t.Errorf("only %d distinct addresses in 32 draws", len(seen))
	}
}
