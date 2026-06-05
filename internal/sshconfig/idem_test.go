package sshconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yourname/sshm/internal/profile"
)

// Strict idempotency: Sync twice with same input -> byte-identical file.
func TestStrictIdempotent(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "config")
	orig := "# my notes\nHost a\n  HostName 1.1.1.1\n\nHost b\n  HostName 2.2.2.2\n"
	os.WriteFile(conf, []byte(orig), 0o600)

	profs := []profile.Profile{
		{Name: "x", HostName: "9.9.9.9", User: "u", Key: "~/.ssh/x"},
		{Name: "y", HostName: "8.8.8.8", Port: 2200},
	}
	if err := Sync(conf, profs); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(conf)
	if err := Sync(conf, profs); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(conf)
	if string(first) != string(second) {
		t.Fatalf("NOT idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	// run a third time for good measure
	Sync(conf, profs)
	third, _ := os.ReadFile(conf)
	if string(second) != string(third) {
		t.Fatal("drift on third run")
	}
}

// Content both BEFORE and AFTER the managed block must be preserved verbatim.
func TestPreservesSurroundingContentExactly(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "config")
	// First, let Sync create the block, then wrap it with hand-written content
	// on both sides and ensure a re-sync keeps both sides byte-for-byte.
	Sync(conf, []profile.Profile{{Name: "m", HostName: "1.1.1.1"}})
	created, _ := os.ReadFile(conf)

	before := "Host top\n  HostName 7.7.7.7\n  # hand comment\n\n"
	after := "\nHost bottom\n  HostName 6.6.6.6\n"
	wrapped := before + string(created) + after
	os.WriteFile(conf, []byte(wrapped), 0o600)

	if err := Sync(conf, []profile.Profile{{Name: "m", HostName: "1.1.1.1"}}); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(conf)
	s := string(out)
	if len(s) < len(before) || s[:len(before)] != before {
		t.Fatalf("content BEFORE block changed:\n%q", s)
	}
	if len(s) < len(after) || s[len(s)-len(after):] != after {
		t.Fatalf("content AFTER block changed:\n%q", s)
	}
}

// No-config case: creating from scratch, then re-sync is idempotent.
func TestFromScratchIdempotent(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "config")
	p := []profile.Profile{{Name: "solo", HostName: "1.2.3.4"}}
	Sync(conf, p)
	a, _ := os.ReadFile(conf)
	Sync(conf, p)
	b, _ := os.ReadFile(conf)
	if string(a) != string(b) {
		t.Fatalf("scratch not idempotent:\n%s\n---\n%s", a, b)
	}
}

// The pristine .sshm.orig must capture the ORIGINAL config and never change,
// even after many syncs with different profiles.
func TestPristineBackupNeverOverwritten(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "config")
	original := "Host precious\n  HostName 1.1.1.1\n  IdentityFile ~/.ssh/precious_key\n"
	os.WriteFile(conf, []byte(original), 0o600)

	Sync(conf, []profile.Profile{{Name: "p1", HostName: "2.2.2.2"}})
	Sync(conf, []profile.Profile{{Name: "p2", HostName: "3.3.3.3"}})
	Sync(conf, []profile.Profile{}) // remove all

	orig, err := os.ReadFile(conf + ".sshm.orig")
	if err != nil {
		t.Fatalf("pristine backup missing: %v", err)
	}
	if string(orig) != original {
		t.Fatalf("pristine backup was altered!\nwant: %q\ngot:  %q", original, orig)
	}
	// And the user's hand-written host is still in the live config.
	live, _ := os.ReadFile(conf)
	if !contains(string(live), "Host precious") || !contains(string(live), "precious_key") {
		t.Fatalf("hand-written host/key reference lost:\n%s", live)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
