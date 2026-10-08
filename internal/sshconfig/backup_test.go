package sshconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yourname/sshm/internal/profile"
)

func TestSyncFailsWhenBackupCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "config")
	original := "Host precious\n  HostName 1.1.1.1\n  IdentityFile ~/.ssh/precious_key\n"
	if err := os.WriteFile(conf, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(conf+".sshm.bak", 0o700); err != nil {
		t.Fatal(err)
	}

	err := Sync(conf, []profile.Profile{{Name: "p1", HostName: "2.2.2.2"}})
	if err == nil {
		t.Fatal("Sync returned nil even though the backup could not be written")
	}

	live, readErr := os.ReadFile(conf)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(live) != original {
		t.Fatalf("config was overwritten despite the failed backup\nwant: %q\ngot:  %q", original, live)
	}
}
