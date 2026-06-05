package authkeys

import "testing"

func TestParsePlain(t *testing.T) {
	e, ok := ParseLine("ssh-ed25519 AAAAC3NzaC1lZDI1 alice@laptop")
	if !ok {
		t.Fatal("should parse")
	}
	if e.Type != "ssh-ed25519" || e.Comment != "alice@laptop" || e.Options != "" {
		t.Fatalf("bad parse: %+v", e)
	}
}

func TestParseWithOptionsContainingSpaces(t *testing.T) {
	line := `command="/usr/bin/backup --now",no-pty ssh-rsa AAAAB3Nza key-for-backups`
	e, ok := ParseLine(line)
	if !ok {
		t.Fatal("should parse")
	}
	if e.Type != "ssh-rsa" {
		t.Fatalf("type not found past options: %+v", e)
	}
	if e.Comment != "key-for-backups" {
		t.Fatalf("comment wrong: %q", e.Comment)
	}
	if e.Options == "" || !contains(e.Options, "backup --now") {
		t.Fatalf("options with spaces lost: %q", e.Options)
	}
}

func TestParseSkipsBlanksAndComments(t *testing.T) {
	body := "\n# a comment\nssh-ed25519 AAAA bob\n\n"
	es := Parse(body)
	if len(es) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(es))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
