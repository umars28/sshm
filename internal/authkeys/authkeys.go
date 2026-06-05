// Package authkeys parses OpenSSH authorized_keys files and can read them
// either locally or from a remote host (over ssh). Each entry tells you which
// public key is allowed to log in, plus any forced options and the comment.
package authkeys

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Entry is one parsed authorized_keys line.
type Entry struct {
	Options string // optional leading options (e.g. no-port-forwarding,...)
	Type    string // ssh-ed25519, ssh-rsa, ecdsa-sha2-nistp256, sk-..., etc.
	Key     string // base64 key material
	Comment string // trailing comment (often user@host)
	Raw     string // original line
}

// Fingerprint-ish short label for display.
func (e Entry) Label() string {
	c := e.Comment
	if c == "" {
		c = "(tanpa komen)"
	}
	short := e.Key
	if len(short) > 12 {
		short = short[:6] + "…" + short[len(short)-6:]
	}
	return fmt.Sprintf("%s %s %s", e.Type, short, c)
}

var keyTypePrefixes = []string{
	"ssh-ed25519", "ssh-rsa", "ssh-dss",
	"ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521",
	"sk-ssh-ed25519@openssh.com", "sk-ecdsa-sha2-nistp256@openssh.com",
}

func isKeyType(tok string) bool {
	for _, p := range keyTypePrefixes {
		if tok == p {
			return true
		}
	}
	return false
}

// ParseLine parses a single authorized_keys line. Returns ok=false for blank
// lines and comments. Options (which may contain spaces inside quotes) are
// detected by finding the first token that is a known key type.
func ParseLine(line string) (Entry, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return Entry{}, false
	}
	e := Entry{Raw: line}

	toks := splitRespectingQuotes(trimmed)
	// Find the key-type token.
	kt := -1
	for i, t := range toks {
		if isKeyType(t) {
			kt = i
			break
		}
	}
	if kt < 0 || kt+1 >= len(toks) {
		// Unrecognized; surface raw so nothing is silently dropped.
		e.Type = "unknown"
		e.Comment = trimmed
		return e, true
	}
	if kt > 0 {
		e.Options = strings.Join(toks[:kt], " ")
	}
	e.Type = toks[kt]
	e.Key = toks[kt+1]
	if kt+2 < len(toks) {
		e.Comment = strings.Join(toks[kt+2:], " ")
	}
	return e, true
}

// splitRespectingQuotes splits on whitespace but keeps quoted substrings
// (authorized_keys options like command="..." may contain spaces).
func splitRespectingQuotes(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case (r == ' ' || r == '\t') && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// Parse parses a full authorized_keys body.
func Parse(body string) []Entry {
	var out []Entry
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // keys can be long
	for sc.Scan() {
		if e, ok := ParseLine(sc.Text()); ok {
			out = append(out, e)
		}
	}
	return out
}

// ReadLocal reads ~/.ssh/authorized_keys for the current user.
func ReadLocal() ([]Entry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".ssh", "authorized_keys")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(string(data)), nil
}

// ReadRemote runs `ssh <alias> cat ~/.ssh/authorized_keys` and parses the
// output. alias is an entry in ~/.ssh/config (a profile name). It uses
// BatchMode so it fails fast instead of hanging on a password prompt.
func ReadRemote(alias string) ([]Entry, error) {
	cmd := exec.Command("ssh",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=8",
		alias, "cat ~/.ssh/authorized_keys")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ssh %s gagal: %v\n%s", alias, err, strings.TrimSpace(string(out)))
	}
	return Parse(string(out)), nil
}
