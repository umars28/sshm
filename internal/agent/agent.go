// Package agent wraps the ssh-agent via the ssh-add CLI. The security model
// the user wants ("re-enter passphrase after restart or sleep") is exactly
// ssh-agent behavior:
//
//   - Restart: the agent process dies, so all keys are gone -> next use of a
//     passphrase-protected key prompts again. Nothing to do.
//   - Sleep: not automatic. We clear the agent (Lock) on sleep so that waking
//     forces the passphrase again. See package sleephook for wiring this.
package agent

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Available reports whether an ssh-agent is reachable (SSH_AUTH_SOCK set).
func Available() bool {
	return os.Getenv("SSH_AUTH_SOCK") != ""
}

// AddKey loads a private key into the agent. ssh-add itself prompts for the
// passphrase on the terminal, so this must run attached to a real TTY (the TUI
// suspends itself while running it). ttlSeconds>0 sets an auto-expiry.
func AddKey(keyPath string, ttlSeconds int) error {
	if keyPath == "" {
		return fmt.Errorf("path key kosong")
	}
	args := []string{}
	if ttlSeconds > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", ttlSeconds))
	}
	args = append(args, expandHome(keyPath))
	cmd := exec.Command("ssh-add", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Lock removes every identity from the agent (`ssh-add -D`). After this, any
// passphrase-protected key must be re-added (re-entering the passphrase).
func Lock() error {
	out, err := exec.Command("ssh-add", "-D").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh-add -D: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Loaded lists the comments/fingerprints of keys currently in the agent.
// Returns an empty slice when the agent holds no identities.
func Loaded() ([]string, error) {
	out, err := exec.Command("ssh-add", "-l").CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		// ssh-add exits 1 when the agent has no identities.
		if strings.Contains(text, "no identities") {
			return nil, nil
		}
		return nil, fmt.Errorf("ssh-add -l: %v: %s", err, text)
	}
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// IsKeyLoaded reports whether a key matching keyPath appears loaded. This is a
// best-effort match on the path/comment shown by `ssh-add -l`.
func IsKeyLoaded(keyPath string) bool {
	loaded, err := Loaded()
	if err != nil {
		return false
	}
	base := expandHome(keyPath)
	for _, l := range loaded {
		if strings.Contains(l, base) {
			return true
		}
	}
	return false
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + p[1:]
		}
	}
	return p
}
