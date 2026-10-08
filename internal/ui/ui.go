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
	"runtime"
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
	modeSearch
	modeConnectOptions
	modeForm
	modeKeys
)

type connectOptions struct {
	passwordAuth   bool
	useProfileUser bool
	useIdentityKey bool
	cursor         int
}

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

	search textinput.Model

	connect connectOptions

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
	search := textinput.New()
	search.Placeholder = "cari..."
	search.Prompt = "/ "
	search.CharLimit = 128

	m := Model{
		store:    store,
		confPath: confPath,
		mode:     modeList,
		search:   search,
		connect: connectOptions{
			passwordAuth:   false,
			useProfileUser: true,
			useIdentityKey: true,
		},
	}
	m.reload()
	return m
}

func (m *Model) reload() {
	m.profiles = filterProfiles(m.store.Sorted(), strings.TrimSpace(m.search.Value()))
	if m.cursor >= len(m.profiles) {
		m.cursor = max(0, len(m.profiles)-1)
	}
}

func filterProfiles(all []profile.Profile, query string) []profile.Profile {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return all
	}
	out := make([]profile.Profile, 0, len(all))
	for _, p := range all {
		hay := strings.ToLower(strings.Join([]string{p.Name, p.HostName, p.User}, " "))
		if strings.Contains(hay, q) {
			out = append(out, p)
		}
	}
	return out
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
			m.setOK("ssh dibuka di tab/terminal baru")
		}
		return m, nil
	case statusMsg:
		if msg.err != nil {
			m.setErr(msg.err.Error())
		} else {
			m.setOK(msg.message)
		}
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeForm:
			return m.updateForm(msg)
		case modeSearch:
			return m.updateSearch(msg)
		case modeConnectOptions:
			return m.updateConnectOptions(msg)
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
			return m, m.connectCmd(p)
		}
	case "/", "s":
		m.mode = modeSearch
		m.search.Focus()
		return m, nil
	case "o":
		if _, ok := m.current(); ok {
			m.mode = modeConnectOptions
			return m, nil
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
func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return m, nil
	case "enter":
		m.mode = modeList
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.reload()
	return m, cmd
}

func (m Model) updateConnectOptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
		return m, nil
	case "up", "k":
		if m.connect.cursor > 0 {
			m.connect.cursor--
		}
	case "down", "j":
		if m.connect.cursor < 2 {
			m.connect.cursor++
		}
	case " ":
		switch m.connect.cursor {
		case 0:
			m.connect.passwordAuth = !m.connect.passwordAuth
		case 1:
			m.connect.useProfileUser = !m.connect.useProfileUser
		case 2:
			m.connect.useIdentityKey = !m.connect.useIdentityKey
		}
	case "enter":
		if p, ok := m.current(); ok {
			m.mode = modeList
			return m, m.connectCmd(p)
		}
	}
	return m, nil
}

type statusMsg struct {
	err     error
	message string
}

func (m Model) connectCmd(p profile.Profile) tea.Cmd {
	c, err := m.buildSSHCommand(p)
	if err != nil {
		return func() tea.Msg { return connectMsg{err} }
	}
	return func() tea.Msg {
		err := openInTerminal(c)
		return connectMsg{err}
	}
}

func (m Model) loadKey(p profile.Profile) tea.Cmd {
	return func() tea.Msg {
		if p.Key == "" {
			return statusMsg{err: fmt.Errorf("profile %s tidak memiliki IdentityFile", p.Name)}
		}
		if err := agent.AddKey(p.Key, 0); err != nil {
			return statusMsg{err: err}
		}
		return statusMsg{message: "key dimuat ke ssh-agent"}
	}
}

func (m Model) persist() error {
	if err := m.store.Save(); err != nil {
		return err
	}
	return sshconfig.Sync(m.confPath, m.store.Sorted())
}

func (m Model) buildSSHCommand(p profile.Profile) (*exec.Cmd, error) {
	args := []string{}
	if m.connect.passwordAuth {
		args = append(args, "-o", "PreferredAuthentications=password,keyboard-interactive", "-o", "PubkeyAuthentication=no")
	}
	if !m.connect.useIdentityKey {
		args = append(args, "-o", "PubkeyAuthentication=no")
	}
	if m.connect.useProfileUser && p.User != "" {
		args = append(args, "-l", p.User)
	}
	if p.Port != 0 {
		args = append(args, "-p", strconv.Itoa(p.Port))
	}
	if m.connect.useProfileUser {
		args = append(args, p.Name)
	} else {
		args = append(args, p.HostName)
	}
	return terminalCommand("ssh", args...)
}

func terminalCommand(cmd string, args ...string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`tell application "Terminal" to do script "%s"`, escapeForAppleScript(cmd, args...))
		return exec.Command("osascript", "-e", script), nil
	case "linux":
		command := fmt.Sprintf("%s %s", cmd, shellEscape(args...))
		if _, err := exec.LookPath("gnome-terminal"); err == nil {
			return exec.Command("gnome-terminal", "--", "bash", "-lc", command), nil
		}
		if _, err := exec.LookPath("konsole"); err == nil {
			return exec.Command("konsole", "--hold", "-e", "bash", "-lc", command), nil
		}
		if _, err := exec.LookPath("x-terminal-emulator"); err == nil {
			return exec.Command("x-terminal-emulator", "-e", command), nil
		}
		return exec.Command(cmd, args...), nil
	default:
		return exec.Command(cmd, args...), nil
	}
}

func openInTerminal(c *exec.Cmd) error {
	if runtime.GOOS == "darwin" {
		return c.Run()
	}
	return c.Start()
}

func shellEscape(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, `'`+strings.ReplaceAll(part, "'", `\'`)+`'`)
	}
	return strings.Join(escaped, " ")
}

func escapeForAppleScript(cmd string, args ...string) string {
	return strings.ReplaceAll(shellEscape(append([]string{cmd}, args...)...), `"`, `\\"`)
}

func (m *Model) setOK(s string)  { m.status, m.isErr = s, false }
func (m *Model) setErr(s string) { m.status, m.isErr = s, true }

func (m Model) View() string {
	switch m.mode {
	case modeForm:
		return m.viewForm()
	case modeSearch:
		return m.viewList()
	case modeConnectOptions:
		return m.viewConnectOptions()
	case modeKeys:
		return m.viewKeys()
	default:
		return m.viewList()
	}
}

func (m Model) viewList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sshm — koneksi tersimpan") + "\n\n")
	if m.search.Value() != "" || m.mode == modeSearch {
		b.WriteString(dimStyle.Render("Search: "+m.search.View()) + "\n\n")
	}
	if len(m.profiles) == 0 {
		if m.search.Value() != "" {
			b.WriteString(dimStyle.Render("  (tidak ada hasil untuk pencarian ini)") + "\n")
		} else {
			b.WriteString(dimStyle.Render("  (belum ada — tekan 'a' untuk menambah)") + "\n")
		}
	}
	for i, p := range m.profiles {
		line := fmt.Sprintf("%s  %s@%s:%d", p.Name, valOr(p.User, "—"), p.HostName, p.SSHPort())
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("connect options: password=%t user=%t key=%t", m.connect.passwordAuth, m.connect.useProfileUser, m.connect.useIdentityKey)) + "\n")
	if m.status != "" {
		st := dimStyle
		if m.isErr {
			st = errStyle
		}
		b.WriteString("\n" + st.Render(m.status) + "\n")
	}
	b.WriteString(helpStyle.Render("enter connect · / search · o options · a add · e edit · d del · l load key · x lock · r/R authorized_keys · q quit"))
	return b.String()
}

func (m Model) viewConnectOptions() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Connect options") + "\n\n")
	options := []struct {
		label string
		on    bool
	}{
		{"Password auth", m.connect.passwordAuth},
		{"Use profile user", m.connect.useProfileUser},
		{"Use identity key", m.connect.useIdentityKey},
	}
	for i, opt := range options {
		marker := "  "
		if m.connect.cursor == i {
			marker = selectedStyle.Render("› ")
		}
		state := "off"
		if opt.on {
			state = "on"
		}
		b.WriteString(marker + fmt.Sprintf("[%s] %s\n", state, opt.label))
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("space toggle · enter connect · esc/q kembali"))
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
