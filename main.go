// Command sshm is a lightweight terminal UI for managing named SSH
// connections, with agent-based locking on sleep/restart and an
// authorized_keys viewer.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yourname/sshm/internal/agent"
	"github.com/yourname/sshm/internal/profile"
	"github.com/yourname/sshm/internal/sleephook"
	"github.com/yourname/sshm/internal/sshconfig"
	"github.com/yourname/sshm/internal/ui"
)

const help = `sshm — SSH connection manager (TUI)

Penggunaan:
  sshm                  Buka TUI (daftar koneksi, connect, kelola key)
  sshm sync             Regenerasi blok terkelola di ~/.ssh/config dari store
  sshm lock             Buang semua key dari ssh-agent (minta passphrase lagi)
  sshm guard            Jalankan penjaga sleep (lock agent saat suspend)
  sshm guard install    Pasang service penjaga (systemd-user / launchd)
  sshm help             Tampilkan bantuan ini

Di dalam TUI: enter=connect a=add e=edit d=del l=load-key x=lock r/R=authorized_keys q=quit
`

func main() {
	sub := ""
	if len(os.Args) > 1 {
		sub = os.Args[1]
	}
	var err error
	switch sub {
	case "", "tui":
		err = runTUI()
	case "sync":
		err = runSync()
	case "lock":
		err = agent.Lock()
		if err == nil {
			fmt.Println("agent di-lock.")
		}
	case "guard":
		if len(os.Args) > 2 && os.Args[2] == "install" {
			err = installGuard()
		} else {
			err = sleephook.RunGuard()
		}
	case "help", "-h", "--help":
		fmt.Print(help)
	default:
		fmt.Printf("perintah tidak dikenal: %s\n\n%s", sub, help)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func load() (*profile.Store, string, error) {
	sp, err := profile.DefaultPath()
	if err != nil {
		return nil, "", err
	}
	st, err := profile.Load(sp)
	if err != nil {
		return nil, "", err
	}
	cp, err := sshconfig.DefaultPath()
	if err != nil {
		return nil, "", err
	}
	return st, cp, nil
}

func runTUI() error {
	st, cp, err := load()
	if err != nil {
		return err
	}
	if !agent.Available() {
		fmt.Fprintln(os.Stderr, "peringatan: ssh-agent tidak terdeteksi (SSH_AUTH_SOCK kosong); fitur load/lock key tidak akan berfungsi.")
	}
	p := tea.NewProgram(ui.New(st, cp), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func runSync() error {
	st, cp, err := load()
	if err != nil {
		return err
	}
	if err := sshconfig.Sync(cp, st.Sorted()); err != nil {
		return err
	}
	fmt.Printf("✓ %s diperbarui (%d koneksi)\n", cp, len(st.Profiles))
	return nil
}

func installGuard() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	switch goos() {
	case "linux":
		path, content, err := sleephook.SystemdUserUnit(self)
		if err != nil {
			return err
		}
		if err := writeFile(path, content); err != nil {
			return err
		}
		fmt.Printf("✓ unit ditulis: %s\n", path)
		fmt.Println("  Aktifkan: systemctl --user daemon-reload && systemctl --user enable --now sshm-guard")
	case "darwin":
		path, content := sleephook.LaunchdPlist(self)
		if err := writeFile(path, content); err != nil {
			return err
		}
		fmt.Printf("✓ plist ditulis: %s\n", path)
		fmt.Println("  Muat: launchctl load " + path)
		fmt.Println("  " + sleephook.InstallHint())
	default:
		fmt.Println(sleephook.InstallHint())
	}
	return nil
}
