// Package profile defines a saved SSH connection ("ec2 project a") and a small
// JSON store. Profiles are the source of truth; the sshconfig package renders
// them into ~/.ssh/config so plain `ssh <name>` works outside the TUI too.
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Profile is one named connection.
type Profile struct {
	Name     string   `json:"name"`               // alias, e.g. "ec2-project-a" (becomes Host)
	HostName string   `json:"hostname"`           // IP or DNS
	User     string   `json:"user,omitempty"`     // login user
	Port     int      `json:"port,omitempty"`     // default 22 when 0
	Key      string   `json:"identity,omitempty"` // path to private key (IdentityFile)
	Tags     []string `json:"tags,omitempty"`     // optional grouping
}

// Validate checks the minimum required fields and normalizes the name.
func (p *Profile) Validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return fmt.Errorf("nama wajib")
	}
	if strings.ContainsAny(p.Name, " \t") {
		return fmt.Errorf("nama tidak boleh mengandung spasi (pakai - atau _)")
	}
	if strings.TrimSpace(p.HostName) == "" {
		return fmt.Errorf("hostname wajib")
	}
	if p.Port < 0 || p.Port > 65535 {
		return fmt.Errorf("port tidak valid")
	}
	return nil
}

// SSHPort returns the effective port (22 if unset).
func (p Profile) SSHPort() int {
	if p.Port == 0 {
		return 22
	}
	return p.Port
}

// Store is the persisted set of profiles.
type Store struct {
	path     string
	Profiles map[string]Profile `json:"profiles"`
}

// DefaultPath returns ~/.config/sshm/profiles.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "sshm", "profiles.json"), nil
}

// Load reads the store (empty if the file does not exist).
func Load(path string) (*Store, error) {
	s := &Store{path: path, Profiles: map[string]Profile{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if s.Profiles == nil {
		s.Profiles = map[string]Profile{}
	}
	s.path = path
	return s, nil
}

// Save writes the store atomically (write temp, then rename).
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Upsert adds or replaces a profile after validation.
func (s *Store) Upsert(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	s.Profiles[p.Name] = p
	return nil
}

// Remove deletes a profile by name.
func (s *Store) Remove(name string) bool {
	if _, ok := s.Profiles[name]; !ok {
		return false
	}
	delete(s.Profiles, name)
	return true
}

// Sorted returns profiles ordered by name.
func (s *Store) Sorted() []Profile {
	out := make([]Profile, 0, len(s.Profiles))
	for _, p := range s.Profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
