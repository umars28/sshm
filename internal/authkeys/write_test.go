package authkeys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLocalRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	in := Parse("ssh-ed25519 AAAA alice@laptop\nssh-rsa BBBB bob@desktop\n")
	if err := WriteLocal(in); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	out := Parse(string(data))
	if len(out) != len(in) {
		t.Fatalf("got %d entries, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i].Key != in[i].Key {
			t.Errorf("entry %d key = %q, want %q", i, out[i].Key, in[i].Key)
		}
	}
}
