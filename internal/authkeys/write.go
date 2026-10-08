package authkeys

import (
	"os"
	"path/filepath"
	"strings"
)

// WriteLocal replaces ~/.ssh/authorized_keys with the given entries.
func WriteLocal(entries []Entry) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.Raw)
		b.WriteString("\n")
	}

	dir := filepath.Join(home, ".ssh")
	_ = os.MkdirAll(dir, 0o755)

	path := filepath.Join(dir, "authorized_keys")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
