// Package ui is the Bubble Tea TUI for sshm. It is deliberately built on a
// small, stable slice of the charm API (tea core + lipgloss + textinput) so it
// stays easy to compile and maintain.
//
// Keys (list view):
//
//	↑/↓ or j/k  move      enter  connect (ssh)
//	a add   e edit   d delete    l load key into agent   x lock agent
//	r read remote authorized_keys   R read local         q quit
package ui

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yourname/sshm/internal/agent"
	"github.com/yourname/sshm/internal/authkeys"
	"github.com/yourname/sshm/internal/profile"
	"github.com/yourname/sshm/internal/sshconfig"
)

type mode int

const (
	modeList mode = iota
	modeForm
	modeKeys
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Faint(true)
	helpStyle     = lipgloss.NewStyle().Faint(true).Padding(1, 1)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

// Model is the root TUI model.
type Model struct {
	store    *profile.Store
	confPath string

	profiles []profile.Profile
	cursor   int

	mode   mode
	status string
	isErr  bool

	// form state
	inputs  []textinput.Model
	editing string // name being edited ("" = new)
	formIdx int

	// keys view
	keysTitle string
	keys      []authkeys.Entry
}

// New builds the initial model.
func New(store *profile.Store, confPath string) Model {
	m := Model{store: store, confPath: confPath, mode: modeList}
	m.reload()
	return m
}

func (m *Model) reload() {
	m.profiles = m.store.Sorted()
	if m.cursor >= len(m.profiles) {
		m.cursor = max(0, len(m.profiles)-1)
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) current() (profile.Profile, bool) {
	if m.cursor < 0 || m.cursor >= len(m.profiles) {
		return profile.Profile{}, false
	}
	return m.profiles[m.cursor], true
}

// connectMsg carries the result of an ssh session back into the TUI.
type connectMsg struct{ err error }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case connectMsg:
		if msg.err != nil {
			m.setErr("ssh selesai dengan error: " + msg.err.Error())
		} else {
			m.setOK("sesi ssh selesai")
		}
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeForm:
			return m.updateForm(msg)
		case modeKeys:
			if msg.String() == "esc" || msg.String() == "q" {
				m.mode = modeList
			}
			return m, nil
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.profiles)-1 {
			m.cursor++
		}
	case "enter":
		if p, ok := m.current(); ok {
			return m, m.connect(p)
		}
	case "a":
		m.startForm(profile.Profile{Port: 22}, "")
	case "e":
		if p, ok := m.current(); ok {
			m.startForm(p, p.Name)
		}
	case "d":
		if p, ok := m.current(); ok {
			m.store.Remove(p.Name)
			if err := m.persist(); err != nil {
				m.setErr(err.Error())
			} else {
				m.setOK("dihapus: " + p.Name)
			}
			m.reload()
		}
	case "l":
		if p, ok := m.current(); ok {
			return m, m.loadKey(p)
		}
	case "x":
		if err := agent.Lock(); err != nil {
			m.setErr(err.Error())
		} else {
			m.setOK("agent di-lock — passphrase diminta lagi nanti")
		}
	case "r":
		if p, ok := m.current(); ok {
			es, err := authkeys.ReadRemote(p.Name)
			if err != nil {
				m.setErr(err.Error())
			} else {
				m.keysTitle = "authorized_keys @ " + p.Name
				m.keys = es
				m.mode = modeKeys
			}
		}
	case "R":
		es, err := authkeys.ReadLocal()
		if err != nil {
			m.setErr(err.Error())
		} else {
			m.keysTitle = "authorized_keys (lokal)"
			m.keys = es
			m.mode = modeKeys
		}
	}
	return m, nil
}

// connect suspends the TUI, runs `ssh <name>` on the real terminal, resumes.
func (m Model) connect(p profile.Profile) tea.Cmd {
	c := exec.Command("ssh", p.Name)
	return tea.ExecProcess(c, func(err error) tea.Msg { return connectMsg{err} })
}

// loadKey runs ssh-add for the profile's key (prompts passphrase on the TTY).
func (m Model) loadKey(p profile.Profile) tea.Cmd {
	if p.Key == "" {
		m2 := m
		m2.setErr("profil ini tidak punya IdentityFile")
		return nil
	}
	c := exec.Command("ssh-add", expand(p.Key))
	return tea.ExecProcess(c, func(err error) tea.Msg { return connectMsg{err} })
}

func (m *Model) persist() error {
	if err := m.store.Save(); err != nil {
		return err
	}
	return sshconfig.Sync(m.confPath, m.store.Sorted())
}

func (m *Model) setOK(s string)  { m.status, m.isErr = s, false }
func (m *Model) setErr(s string) { m.status, m.isErr = s, true }

func (m Model) View() string {
	switch m.mode {
	case modeForm:
		return m.viewForm()
	case modeKeys:
		return m.viewKeys()
	default:
		return m.viewList()
	}
}

func (m Model) viewList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sshm — koneksi tersimpan") + "\n\n")
	if len(m.profiles) == 0 {
		b.WriteString(dimStyle.Render("  (belum ada — tekan 'a' untuk menambah)") + "\n")
	}
	for i, p := range m.profiles {
		line := fmt.Sprintf("%s  %s@%s:%d", p.Name, valOr(p.User, "—"), p.HostName, p.SSHPort())
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	if m.status != "" {
		st := dimStyle
		if m.isErr {
			st = errStyle
		}
		b.WriteString("\n" + st.Render(m.status) + "\n")
	}
	b.WriteString(helpStyle.Render("enter connect · a add · e edit · d del · l load key · x lock · r/R authorized_keys · q quit"))
	return b.String()
}

func (m Model) viewKeys() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.keysTitle) + "\n\n")
	if len(m.keys) == 0 {
		b.WriteString(dimStyle.Render("  (kosong)") + "\n")
	}
	for _, e := range m.keys {
		opt := ""
		if e.Options != "" {
			opt = dimStyle.Render("  [" + e.Options + "]")
		}
		b.WriteString("  • " + e.Label() + opt + "\n")
	}
	b.WriteString(helpStyle.Render("esc/q kembali"))
	return b.String()
}

func valOr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := homeDir(); err == nil {
			return h + p[1:]
		}
	}
	return p
}

// small indirections kept here so the form file stays focused
func atoiSafe(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// declared here, implemented via os in form.go to avoid extra imports churn
var homeDir = func() (string, error) { return userHome() }
