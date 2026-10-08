package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	body := `{"profiles":{"ec2-project-a":{"name":"ec2-project-a","hostname":"1.2.3.4","user":"ubuntu","port":2222,"identity":"~/.ssh/a.pem","tags":["prod"]}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d: %+v", len(s.Profiles), s.Profiles)
	}
	p, ok := s.Profiles["ec2-project-a"]
	if !ok {
		t.Fatalf("profile not keyed by name: %+v", s.Profiles)
	}
	if p.HostName != "1.2.3.4" || p.User != "ubuntu" || p.Port != 2222 || p.Key != "~/.ssh/a.pem" {
		t.Fatalf("fields not decoded: %+v", p)
	}
	if len(p.Tags) != 1 || p.Tags[0] != "prod" {
		t.Fatalf("tags not decoded: %+v", p.Tags)
	}
}

func TestLoadMissingFileGivesUsableEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "profiles.json")

	s, err := Load(path)
	if err != nil {
		t.Fatalf("missing file should not be an error: %v", err)
	}
	if s == nil {
		t.Fatal("store should not be nil")
	}
	if len(s.Profiles) != 0 {
		t.Fatalf("expected empty store, got %+v", s.Profiles)
	}

	if err := s.Upsert(Profile{Name: "db", HostName: "10.0.0.2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("store from missing file should remember its path: %v", err)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Profiles["db"]; !ok {
		t.Fatalf("profile did not round-trip to %s: %+v", path, again.Profiles)
	}
}

func TestLoadMalformedContent(t *testing.T) {
	cases := map[string]string{
		"truncated object": `{"profiles":{"db":{"name":"db"`,
		"not json":         "this is not json at all",
		"profiles is list": `{"profiles":["db"]}`,
		"empty file":       "",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "profiles.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := Load(path)
			if err == nil {
				t.Fatalf("expected an error for %q, got store %+v", body, s.Profiles)
			}
			if s != nil {
				t.Fatalf("store should be nil when the file cannot be parsed: %+v", s)
			}
		})
	}
}

func TestLoadNullProfilesYieldsNonNilMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte(`{"profiles":null}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Profiles == nil {
		t.Fatal("Profiles must be usable, not a nil map")
	}
	if err := s.Upsert(Profile{Name: "db", HostName: "10.0.0.2"}); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultPathHonoursHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "sshm", "profiles.json")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDefaultPathWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")

	got, err := DefaultPath()
	if err == nil {
		t.Fatalf("expected an error when HOME is unset, got %q", got)
	}
	if got != "" {
		t.Fatalf("expected an empty path on error, got %q", got)
	}
}
