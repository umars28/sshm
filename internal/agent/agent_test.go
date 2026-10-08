package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeSSHAdd(t *testing.T, output string, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	redirect := ""
	if exitCode != 0 {
		redirect = " >&2"
	}
	script := fmt.Sprintf("#!/bin/sh\n/bin/cat <<'SSHADDEOF'%s\n%s\nSSHADDEOF\nexit %d\n", redirect, output, exitCode)
	if err := os.WriteFile(filepath.Join(dir, "ssh-add"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ssh-add: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestLoadedParsesFingerprintLines(t *testing.T) {
	fakeSSHAdd(t, "256 SHA256:7xQ1aB2cD3eF4gH5iJ6kL7mN8oP9qR0sT1uV2wX3yZ4 /home/alice/.ssh/id_ed25519 (ED25519)\n3072 SHA256:aB1cD2eF3gH4iJ5kL6mN7oP8qR9sT0uV1wX2yZ3aB4c /home/alice/.ssh/id_rsa (RSA)", 0)

	got, err := Loaded()
	if err != nil {
		t.Fatalf("Loaded: %v", err)
	}
	want := []string{
		"256 SHA256:7xQ1aB2cD3eF4gH5iJ6kL7mN8oP9qR0sT1uV2wX3yZ4 /home/alice/.ssh/id_ed25519 (ED25519)",
		"3072 SHA256:aB1cD2eF3gH4iJ5kL6mN7oP8qR9sT0uV1wX2yZ3aB4c /home/alice/.ssh/id_rsa (RSA)",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d lines, got %d: %q", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadedReturnsNothingWhenAgentHasNoIdentities(t *testing.T) {
	fakeSSHAdd(t, "The agent has no identities.", 1)

	got, err := Loaded()
	if err != nil {
		t.Fatalf("empty agent is not an error, got: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no entries, got %q", got)
	}
}

func TestLoadedErrorsWhenAgentUnavailable(t *testing.T) {
	fakeSSHAdd(t, "Error connecting to agent: No such file or directory", 2)

	got, err := Loaded()
	if err == nil {
		t.Fatalf("expected an error when the agent is unreachable, got %q", got)
	}
	if got != nil {
		t.Fatalf("expected nil entries alongside the error, got %q", got)
	}
	if !strings.Contains(err.Error(), "Error connecting to agent") {
		t.Fatalf("error should carry the ssh-add diagnostic, got %q", err)
	}
}

func TestIsKeyLoadedMatchesPathInFingerprintLine(t *testing.T) {
	fakeSSHAdd(t, "256 SHA256:7xQ1aB2cD3eF4gH5iJ6kL7mN8oP9qR0sT1uV2wX3yZ4 /home/alice/.ssh/id_ed25519 (ED25519)\n3072 SHA256:aB1cD2eF3gH4iJ5kL6mN7oP8qR9sT0uV1wX2yZ3aB4c /home/alice/.ssh/id_rsa (RSA)", 0)

	if !IsKeyLoaded("/home/alice/.ssh/id_rsa") {
		t.Fatal("key present in ssh-add -l output should report as loaded")
	}
}

func TestIsKeyLoadedExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	fakeSSHAdd(t, "256 SHA256:7xQ1aB2cD3eF4gH5iJ6kL7mN8oP9qR0sT1uV2wX3yZ4 "+home+"/.ssh/id_ed25519 (ED25519)", 0)

	if !IsKeyLoaded("~/.ssh/id_ed25519") {
		t.Fatal("~ should expand to the home dir before matching")
	}
}

func TestIsKeyLoadedFalseForKeyNotInAgent(t *testing.T) {
	fakeSSHAdd(t, "256 SHA256:7xQ1aB2cD3eF4gH5iJ6kL7mN8oP9qR0sT1uV2wX3yZ4 /home/alice/.ssh/id_ed25519 (ED25519)", 0)

	if IsKeyLoaded("/home/alice/.ssh/id_deploy") {
		t.Fatal("key absent from the agent should not report as loaded")
	}
}

func TestIsKeyLoadedFalseWhenAgentUnavailable(t *testing.T) {
	fakeSSHAdd(t, "Error connecting to agent: No such file or directory", 2)

	if IsKeyLoaded("/home/alice/.ssh/id_ed25519") {
		t.Fatal("unreachable agent should report not loaded, not loaded-by-default")
	}
}

func TestIsKeyLoadedCannotMatchWhenOnlyFingerprintIsShown(t *testing.T) {
	fakeSSHAdd(t, "256 SHA256:7xQ1aB2cD3eF4gH5iJ6kL7mN8oP9qR0sT1uV2wX3yZ4 no comment (ED25519)", 0)

	if IsKeyLoaded("/home/alice/.ssh/id_ed25519") {
		t.Fatal("matching is path-based; a commentless line must not match")
	}
}

func TestAvailableFollowsAuthSock(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/ssh-XXXX/agent.123")
	if !Available() {
		t.Fatal("SSH_AUTH_SOCK set should mean available")
	}

	t.Setenv("SSH_AUTH_SOCK", "")
	if Available() {
		t.Fatal("empty SSH_AUTH_SOCK should mean unavailable")
	}
}

func TestLoadedSkipsBlankLines(t *testing.T) {
	fakeSSHAdd(t, "\n256 SHA256:7xQ1aB2cD3eF4gH5 /home/alice/.ssh/id_ed25519 (ED25519)\n\n   \n3072 SHA256:aB1cD2eF3gH4iJ5k /home/alice/.ssh/id_rsa (RSA)\n", 0)

	got, err := Loaded()
	if err != nil {
		t.Fatalf("Loaded: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries after dropping blanks, got %d: %q", len(got), got)
	}
}
