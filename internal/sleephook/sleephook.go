// Package sleephook implements the "lock on sleep" behavior. A small guard
// process listens for the OS suspend event and clears the ssh-agent so that
// waking the laptop forces the key passphrase to be entered again.
//
// Linux: subscribe to logind's PrepareForSleep signal via `dbus-monitor`
// (present on most desktop installs) and lock on the "true" (about-to-sleep)
// edge. macOS: ship a launchd plist that runs `sshm lock` on wake, paired with
// the lightweight `sleepwatcher` tool (or any wake trigger).
package sleephook

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yourname/sshm/internal/agent"
)

// RunGuard blocks, watching for suspend events and locking the agent on each.
// It is meant to run as a per-user background service (so it inherits the
// user's SSH_AUTH_SOCK). Returns an error if the platform is unsupported or a
// required tool is missing.
func RunGuard() error {
	switch runtime.GOOS {
	case "linux":
		return runGuardLinux()
	default:
		return fmt.Errorf("guard otomatis belum didukung di %s — pakai `sshm lock` lewat hook OS (lihat InstallHint)", runtime.GOOS)
	}
}

func runGuardLinux() error {
	if _, err := exec.LookPath("dbus-monitor"); err != nil {
		return fmt.Errorf("butuh `dbus-monitor` (paket dbus). Alternatif: panggil `sshm lock` dari systemd-sleep hook")
	}
	cmd := exec.Command("dbus-monitor", "--system",
		"type='signal',interface='org.freedesktop.login1.Manager',member='PrepareForSleep'")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Println("sshm guard aktif — agent akan di-lock saat sleep.")
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := sc.Text()
		// The boolean arg follows the signal; "true" == entering sleep.
		if strings.Contains(line, "boolean true") {
			if err := agent.Lock(); err != nil {
				fmt.Fprintln(os.Stderr, "sshm: gagal lock agent:", err)
			} else {
				fmt.Println("sshm: agent di-lock (sleep).")
			}
		}
	}
	return cmd.Wait()
}

// SystemdUserUnit returns the path and content of a systemd *user* service that
// keeps the guard running across the login session.
func SystemdUserUnit(selfPath string) (path, content string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	path = filepath.Join(home, ".config", "systemd", "user", "sshm-guard.service")
	content = fmt.Sprintf(`[Unit]
Description=sshm sleep guard (lock ssh-agent on suspend)
After=default.target

[Service]
ExecStart=%s guard
Restart=on-failure

[Install]
WantedBy=default.target
`, selfPath)
	return path, content, nil
}

// LaunchdPlist returns the path and content of a macOS launchd agent that runs
// `sshm lock` (e.g. triggered on wake by sleepwatcher, or on load).
func LaunchdPlist(selfPath string) (path, content string) {
	home, _ := os.UserHomeDir()
	path = filepath.Join(home, "Library", "LaunchAgents", "com.sshm.guard.plist")
	content = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.sshm.guard</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>lock</string></array>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`, selfPath)
	return path, content
}

// InstallHint returns human instructions for wiring the guard on this OS.
func InstallHint() string {
	switch runtime.GOOS {
	case "linux":
		return "Linux: `sshm guard install` lalu `systemctl --user enable --now sshm-guard`."
	case "darwin":
		return "macOS: `brew install sleepwatcher`, set wake-script ke `sshm lock` (atau pakai launchd plist dari `sshm guard install`)."
	default:
		return "Jalankan `sshm lock` dari hook sleep/wake OS-mu."
	}
}
