package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/j0urneyk/herdrctx/internal/herdr"
	"github.com/j0urneyk/herdrctx/internal/preferences"
)

type machineMenu struct {
	rows     []herdr.Machine
	cursor   int
	conflict string
	previous string
}
type machinesLoadedMsg struct {
	Rows []herdr.Machine
	Err  error
}

func (m model) importMachinesCmd() tea.Cmd {
	m.hostMenu.saving = true
	return func() tea.Msg {
		rows, err := m.clientForLocal().Machines(m.ctx)
		return machinesLoadedMsg{Rows: rows, Err: err}
	}
}

func (m model) handleMachinesLoaded(msg machinesLoadedMsg) (tea.Model, tea.Cmd) {
	m.hostMenu.saving = false
	if msg.Err != nil {
		m.hostMenu.err = msg.Err.Error()
		return m, nil
	}
	if len(msg.Rows) == 0 {
		m.hostMenu.err = "No machine profiles found."
		return m, nil
	}
	m.machineMenu = &machineMenu{rows: msg.Rows}
	return m, nil
}

func (m model) machineMenuView() string {
	menu := m.machineMenu
	var b strings.Builder
	b.WriteString("Import a machine as a saved host\n\n")
	first, last := menuRange(len(menu.rows), menu.cursor, m.height)
	fmt.Fprintf(&b, "Showing %d–%d of %d\n", first+1, last, len(menu.rows))
	for i := first; i < last; i++ {
		row := menu.rows[i]
		prefix := "  "
		if i == menu.cursor {
			prefix = "> "
		}
		state := "enabled"
		if !row.Enabled {
			state = "disabled"
		}
		fmt.Fprintf(&b, "%s%s · %s · session %s · %s\n", prefix, sanitizeDisplay(row.Label), row.Target, row.Session, state)
	}
	b.WriteString("\nEnter previews registration · Esc returns\nRegistering a host permits listing all its sessions.\nThe profile session remains metadata; import never attaches.")
	if menu.conflict != "" {
		b.WriteString("\n" + menu.conflict + "\ny confirms replacement · Esc cancels")
	}
	return dialogBoxStyle.Width(navigationWidth(m.width)).Render(b.String())
}

func handleMachinesInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	menu := m.machineMenu
	switch msg.String() {
	case "esc", "q":
		if menu.conflict != "" {
			menu.conflict = ""
			// A later candidate must not inherit the cancelled replacement target.
			menu.previous = ""
		} else {
			m.machineMenu = nil
		}
	case "up", "k":
		if menu.conflict == "" {
			menu.cursor = max(0, menu.cursor-1)
		}
	case "down", "j":
		if menu.conflict == "" {
			menu.cursor = min(len(menu.rows)-1, menu.cursor+1)
		}
	case "enter", "y":
		row := menu.rows[menu.cursor]
		if !row.Enabled {
			m.hostMenu.err = "Disabled profiles cannot be imported."
			m.machineMenu = nil
			return inputConsumed(m, nil)
		}
		h := preferences.Host{Label: row.Label, Destination: row.Target, ProfileID: row.ID, Session: row.Session}
		if menu.conflict != "" && msg.String() != "y" {
			return inputConsumed(m, nil)
		}
		if menu.conflict == "" {
			var matches []preferences.Host
			for _, old := range m.hostData.Items {
				if old.ProfileID == row.ID || old.Destination == row.Target || old.Label == row.Label {
					matches = append(matches, old)
				}
			}
			if len(matches) > 1 {
				m.hostMenu.err = "This profile conflicts with multiple hosts; resolve them in Hosts first."
				m.machineMenu = nil
				return inputConsumed(m, nil)
			}
			if len(matches) == 1 {
				old := matches[0]
				if old == h {
					m.hostMenu.err = "This profile is already registered."
					m.machineMenu = nil
					return inputConsumed(m, nil)
				}
				menu.previous = old.Destination
				menu.conflict = fmt.Sprintf("Replace %s (%s, session %s) with %s (%s, session %s)?", old.Label, old.Destination, old.Session, h.Label, h.Destination, h.Session)
				return inputConsumed(m, nil)
			}
		}
		m.machineMenu = nil
		next, cmd := m.saveHost(preferences.HostChange{Previous: menu.previous, Host: h})
		return inputConsumed(next.(model), cmd)
	}
	return inputConsumed(m, nil)
}
