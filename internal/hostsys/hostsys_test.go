package hostsys

import (
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	good := []string{"PC-0001", "WORKSTATION", "a_b-1", "N1"}
	for _, s := range good {
		if err := ValidateName(s); err != nil {
			t.Errorf("%q rejected: %v", s, err)
		}
	}
	bad := []string{"", "1234", "有中文", "name with space", "a.b", strings.Repeat("A", 16), "bad!"}
	for _, s := range bad {
		if err := ValidateName(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestValidateWorkgroup(t *testing.T) {
	if err := ValidateWorkgroup("WORKGROUP"); err != nil {
		t.Errorf("WORKGROUP rejected: %v", err)
	}
	for _, s := range []string{"", "a b", `a\b`, strings.Repeat("W", 16)} {
		if err := ValidateWorkgroup(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestRandomName(t *testing.T) {
	for i := 0; i < 50; i++ {
		name := RandomName()
		if err := ValidateName(name); err != nil {
			t.Fatalf("RandomName produced %q: %v", name, err)
		}
	}
}
