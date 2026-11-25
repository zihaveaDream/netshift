package sysnet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseToggleAddDeleteHosts(t *testing.T) {
	sample := "# Copyright comment\r\n\r\n127.0.0.1 localhost\r\n# 10.0.0.5 api.internal # staging\r\n# just a note line\r\n"
	hf := ParseHosts("hosts", sample)
	if len(hf.Entries) != 2 {
		t.Fatalf("parsed %d entries, want 2: %+v", len(hf.Entries), hf.Entries)
	}
	if !hf.Entries[0].Enabled || hf.Entries[0].IP != "127.0.0.1" || hf.Entries[0].Host != "localhost" {
		t.Errorf("entry 0 = %+v", hf.Entries[0])
	}
	if hf.Entries[1].Enabled || hf.Entries[1].IP != "10.0.0.5" || hf.Entries[1].Comment != "staging" {
		t.Errorf("entry 1 = %+v", hf.Entries[1])
	}

	// Enable entry 1, add a new entry, delete entry 0.
	hf.Entries[1].Enabled = true
	if err := hf.Add("192.168.1.50", "db.local", "prod"); err != nil {
		t.Fatal(err)
	}
	hf.Delete(0)

	rendered := hf.Render()
	if !strings.Contains(rendered, "# Copyright comment") || !strings.Contains(rendered, "# just a note line") {
		t.Errorf("prose comments lost:\n%s", rendered)
	}
	if strings.Contains(rendered, "127.0.0.1") {
		t.Errorf("deleted entry still present:\n%s", rendered)
	}
	if !strings.Contains(rendered, "10.0.0.5\tapi.internal # staging") {
		t.Errorf("toggled entry missing:\n%s", rendered)
	}
	if !strings.Contains(rendered, "192.168.1.50\tdb.local # prod") {
		t.Errorf("added entry missing:\n%s", rendered)
	}
}

func TestHostsSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	if err := os.WriteFile(path, []byte("127.0.0.1 localhost\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hf, err := LoadHosts(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := hf.Add("10.1.1.1", "test.local", ""); err != nil {
		t.Fatal(err)
	}
	if err := hf.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadHosts(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Entries) != 2 {
		t.Fatalf("reloaded %d entries, want 2", len(reloaded.Entries))
	}
	if _, err := os.Stat(path + ".netshift.bak"); err != nil {
		t.Error("backup file was not created")
	}
}
