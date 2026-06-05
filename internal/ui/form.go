package ui

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yourname/sshm/internal/profile"
)

const (
	fName = iota
	fHost
	fUser
	fPort
	fKey
	fCount
)

var fieldLabels = []string{"Nama (alias)", "HostName (IP/DNS)", "User", "Port", "IdentityFile (key)"}

// startForm initializes the input fields for adding (editing=="") or editing.
func (m *Model) startForm(p profile.Profile, editing string) {
	m.inputs = make([]textinput.Model, fCount)
	for i := range m.inputs {
		ti := textinput.New()
		ti.Placeholder = fieldLabels[i]
		switch i {
		case fName:
			ti.SetValue(p.Name)
		case fHost:
			ti.SetValue(p.HostName)
		case fUser:
			ti.SetValue(p.User)
		case fPort:
			if p.Port != 0 {
				ti.SetValue(strconv.Itoa(p.Port))
			}
		case fKey:
			ti.SetValue(p.Key)
		}
		m.inputs[i] = ti
	}
	m.formIdx = 0
	m.inputs[0].Focus()
	m.editing = editing
	m.mode = modeForm
	m.status = ""
}

func (m Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return m, nil
	case "tab", "down", "enter":
		// enter on the last field submits; otherwise advance.
		if msg.String() == "enter" && m.formIdx == fCount-1 {
			return m.submitForm()
		}
		m.focusNext(1)
		return m, nil
	case "shift+tab", "up":
		m.focusNext(-1)
		return m, nil
	case "ctrl+s":
		return m.submitForm()
	}
	// pass the key to the focused input
	var cmd tea.Cmd
	m.inputs[m.formIdx], cmd = m.inputs[m.formIdx].Update(msg)
	return m, cmd
}

func (m *Model) focusNext(delta int) {
	m.inputs[m.formIdx].Blur()
	m.formIdx = (m.formIdx + delta + fCount) % fCount
	m.inputs[m.formIdx].Focus()
}

func (m Model) submitForm() (tea.Model, tea.Cmd) {
	p := profile.Profile{
		Name:     strings.TrimSpace(m.inputs[fName].Value()),
		HostName: strings.TrimSpace(m.inputs[fHost].Value()),
		User:     strings.TrimSpace(m.inputs[fUser].Value()),
		Port:     atoiSafe(m.inputs[fPort].Value()),
		Key:      strings.TrimSpace(m.inputs[fKey].Value()),
	}
	// If renaming, drop the old entry.
	if m.editing != "" && m.editing != p.Name {
		m.store.Remove(m.editing)
	}
	if err := m.store.Upsert(p); err != nil {
		m.setErr(err.Error())
		return m, nil // stay in form
	}
	if err := m.persist(); err != nil {
		m.setErr(err.Error())
		return m, nil
	}
	m.setOK("disimpan: " + p.Name)
	m.mode = modeList
	m.reload()
	// place cursor on the saved profile
	for i, pp := range m.profiles {
		if pp.Name == p.Name {
			m.cursor = i
		}
	}
	return m, nil
}

func (m Model) viewForm() string {
	var b strings.Builder
	title := "Tambah koneksi"
	if m.editing != "" {
		title = "Edit: " + m.editing
	}
	b.WriteString(titleStyle.Render(title) + "\n\n")
	for i := range m.inputs {
		marker := "  "
		if i == m.formIdx {
			marker = selectedStyle.Render("› ")
		}
		b.WriteString(marker + dimStyle.Render(fieldLabels[i]+": ") + m.inputs[i].View() + "\n")
	}
	if m.status != "" && m.isErr {
		b.WriteString("\n" + errStyle.Render(m.status) + "\n")
	}
	b.WriteString(helpStyle.Render("tab/↑↓ pindah field · enter di field terakhir / ctrl+s simpan · esc batal"))
	return b.String()
}

func userHome() (string, error) { return os.UserHomeDir() }
