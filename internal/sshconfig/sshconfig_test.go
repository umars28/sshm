package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourname/sshm/internal/profile"
)

func TestSyncPreservesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "config")
	// Pre-existing hand-written content must survive untouched.
	orig := "Host myserver\n    HostName 9.9.9.9\n    User me\n"
	os.WriteFile(conf, []byte(orig), 0o600)

	profs := []profile.Profile{
		{Name: "ec2-project-a", HostName: "1.2.3.4", User: "ubuntu", Key: "~/.ssh/a.pem"},
		{Name: "db", HostName: "10.0.0.2", Port: 2222},
	}
	if err := Sync(conf, profs); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(conf)
	s := string(out)
	if !strings.Contains(s, "Host myserver") {
		t.Fatal("hand-written host was lost")
	}
	if !strings.Contains(s, "Host ec2-project-a") || !strings.Contains(s, "IdentityFile ~/.ssh/a.pem") {
		t.Fatalf("managed host missing:\n%s", s)
	}
	if !strings.Contains(s, "Port 2222") {
		t.Fatal("non-default port not written")
	}

	// Re-sync with one fewer profile: block replaced, not duplicated.
	if err := Sync(conf, profs[:1]); err != nil {
		t.Fatal(err)
	}
	out2, _ := os.ReadFile(conf)
	s2 := string(out2)
	if strings.Count(s2, beginMarker) != 1 {
		t.Fatalf("managed block duplicated:\n%s", s2)
	}
	if strings.Contains(s2, "Host db") {
		t.Fatal("removed profile still present")
	}
	if !strings.Contains(s2, "Host myserver") {
		t.Fatal("hand-written host lost on re-sync")
	}
}

func TestListHostsFlagsManaged(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "config")
	os.WriteFile(conf, []byte("Host plain\n  HostName 5.5.5.5\n"), 0o600)
	Sync(conf, []profile.Profile{{Name: "managed1", HostName: "1.1.1.1"}})

	hosts, err := ListHosts(conf)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, h := range hosts {
		got[h.Name] = h.Managed
	}
	if _, ok := got["plain"]; !ok || got["plain"] {
		t.Fatalf("plain host should be unmanaged: %v", got)
	}
	if m, ok := got["managed1"]; !ok || !m {
		t.Fatalf("managed1 should be flagged managed: %v", got)
	}
}
