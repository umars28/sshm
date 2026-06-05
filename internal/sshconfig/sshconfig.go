// Package sshconfig keeps ~/.ssh/config in sync with the profile store by
// owning a single clearly-delimited block. Everything outside the markers is
// left untouched, so the user's hand-written config is safe.
package sshconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourname/sshm/internal/profile"
)

const (
	beginMarker = "# >>> sshm managed (do not edit this block) >>>"
	endMarker   = "# <<< sshm managed <<<"
)

// DefaultPath returns ~/.ssh/config.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

// render builds the managed block from the profiles.
func render(profiles []profile.Profile) string {
	var b strings.Builder
	b.WriteString(beginMarker + "\n")
	for _, p := range profiles {
		fmt.Fprintf(&b, "Host %s\n", p.Name)
		fmt.Fprintf(&b, "    HostName %s\n", p.HostName)
		if p.User != "" {
			fmt.Fprintf(&b, "    User %s\n", p.User)
		}
		if p.Port != 0 && p.Port != 22 {
			fmt.Fprintf(&b, "    Port %d\n", p.Port)
		}
		if p.Key != "" {
			fmt.Fprintf(&b, "    IdentityFile %s\n", p.Key)
			// IdentitiesOnly avoids the agent offering unrelated keys first.
			b.WriteString("    IdentitiesOnly yes\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(endMarker + "\n")
	return b.String()
}

// Sync rewrites the managed block of confPath to match profiles. The file is
// created (0600, with ~/.ssh at 0700) if missing. A timestamped backup is made
// whenever an existing file is modified.
func Sync(confPath string, profiles []profile.Profile) error {
	dir := filepath.Dir(confPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	existing, err := os.ReadFile(confPath)
	if os.IsNotExist(err) {
		existing = nil
	} else if err != nil {
		return err
	}

	block := render(profiles)
	var out string
	if b, e, found := locateBlock(string(existing)); found {
		// Replace in place.
		out = string(existing[:b]) + block + string(existing[e:])
	} else {
		// Append, keeping a separating newline.
		prefix := string(existing)
		if prefix != "" && !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		if prefix != "" {
			prefix += "\n"
		}
		out = prefix + block
	}

	// No-op guard: if nothing actually changed, don't touch the file or
	// create backups at all.
	if string(existing) == out {
		return nil
	}

	if len(existing) > 0 {
		// Pristine backup: written ONCE and never overwritten, so the very
		// first pre-sshm config is always recoverable.
		orig := confPath + ".sshm.orig"
		if _, err := os.Stat(orig); os.IsNotExist(err) {
			_ = os.WriteFile(orig, existing, 0o600)
		}
		// Rolling backup: the state immediately before this change.
		_ = os.WriteFile(confPath+".sshm.bak", existing, 0o600)
	}
	return os.WriteFile(confPath, []byte(out), 0o600)
}

// locateBlock returns the byte range [begin,end) covering the managed block
// (including markers and trailing newline), and whether it was found.
func locateBlock(s string) (int, int, bool) {
	bi := strings.Index(s, beginMarker)
	if bi < 0 {
		return 0, 0, false
	}
	ei := strings.Index(s[bi:], endMarker)
	if ei < 0 {
		return 0, 0, false
	}
	end := bi + ei + len(endMarker)
	// consume the trailing newline if present
	if end < len(s) && s[end] == '\n' {
		end++
	}
	return bi, end, true
}

// HostAlias is a Host entry discovered in the config (managed or not).
type HostAlias struct {
	Name    string
	Managed bool
}

// ListHosts scans confPath for `Host` aliases, flagging which fall inside the
// managed block. Wildcard hosts (containing * or ?) are skipped. This lets the
// TUI show the user's pre-existing hosts read-only alongside managed ones.
func ListHosts(confPath string) ([]HostAlias, error) {
	data, err := os.ReadFile(confPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	mb, me, hasBlock := locateBlock(string(data))

	var out []HostAlias
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	offset := 0
	for sc.Scan() {
		line := sc.Text()
		lineStart := offset
		offset += len(line) + 1 // +1 for the newline scanner strips

		t := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(t), "host ") {
			continue
		}
		fields := strings.Fields(t)[1:] // drop "Host"
		inManaged := hasBlock && lineStart >= mb && lineStart < me
		for _, name := range fields {
			if strings.ContainsAny(name, "*?") {
				continue
			}
			out = append(out, HostAlias{Name: name, Managed: inManaged})
		}
	}
	return out, sc.Err()
}
