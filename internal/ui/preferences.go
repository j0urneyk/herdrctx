package ui

import (
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/j0urneyk/herdrctx/internal/herdr"
	"github.com/j0urneyk/herdrctx/internal/preferences"
)

type preferencesLoadedMsg struct {
	ID      uint64
	Data    preferences.Data
	Err     error
	Initial bool
}

func (m model) initialPreferencesCmd() tea.Cmd {
	if m.preferencesStore == nil && m.preferencesError == nil {
		return nil
	}
	return func() tea.Msg {
		if m.preferencesError != nil {
			return preferencesLoadedMsg{Initial: true, Err: m.preferencesError}
		}
		d, err := m.preferencesStore.Load()
		return preferencesLoadedMsg{Initial: true, Data: d, Err: err}
	}
}

func (m *model) saveFavoriteCmd(change preferences.Change) tea.Cmd {
	m.preferencesRequestID++
	id, store := m.preferencesRequestID, m.preferencesStore
	m.preferencesPending = true
	return func() tea.Msg {
		d, err := store.Apply(change)
		return preferencesLoadedMsg{ID: id, Data: d, Err: err}
	}
}

func (m model) handlePreferencesLoaded(msg preferencesLoadedMsg) (tea.Model, tea.Cmd) {
	if !msg.Initial && msg.ID != m.preferencesRequestID {
		return m, nil
	}
	if msg.Initial && m.preferencesRequestID != 0 {
		return m, nil
	}
	m.preferencesPending = false
	name, cursor := m.selectedSessionSnapshot()
	if msg.Err != nil {
		if msg.Initial {
			m.preferencesError = msg.Err
		}
		kind := dialogError
		if msg.Initial {
			kind = dialogWarning
		}
		m.showDialog(kind, "Preferences unavailable", msg.Err.Error())
		return m, nil
	}
	m.preferences = msg.Data
	m.configureTablePreserving(name, cursor)
	if !msg.Initial {
		m.setStatus("Preferences saved.", statusSuccess)
	}
	m.resizeNavigation()
	return m, nil
}

func (m *model) preferencesAvailable() bool {
	if m.preferencesPending {
		return false
	}
	if m.preferencesStore == nil || m.preferencesError != nil {
		body := "Preferences storage is unavailable. Restart after fixing the preferences file."
		if m.preferencesError != nil {
			body += "\n\n" + m.preferencesError.Error()
		}
		m.showDialog(dialogWarning, "Preferences unavailable", body)
		return false
	}
	return true
}

func (m model) toggleFavorite() (tea.Model, tea.Cmd) {
	if !m.preferencesAvailable() {
		return m, nil
	}
	if _, _, child := m.selectedAgent(); child {
		m.showDialog(dialogWarning, "Select a session", "Favorites apply to session rows. Move to a session row first.")
		return m, nil
	}
	s, ok := m.selectedSession()
	if !ok {
		m.showDialog(dialogWarning, "No session selected", "Select a session before changing favorites.")
		return m, nil
	}
	cmd := m.saveFavoriteCmd(preferences.Change{Target: s.Target, FavoriteName: s.Name, Favorite: !m.preferences.FavoriteSession(s.ID())})
	return m, cmd
}

func (m model) navigationRows(sessions []herdr.Session, cursor int) []table.Row {
	rows := make([]table.Row, 0, len(sessions))
	for _, s := range sessions {
		name := "▾ " + s.DisplayName()
		if m.preferences.FavoriteSession(s.ID()) {
			name = "▾ ⭐\ufe0f " + s.DisplayName()
		}
		status := s.Status()
		if m.hosts != nil {
			if h := m.hosts.entries[s.Target]; h != nil && h.stale {
				status += " ?"
			}
		}
		context := ""
		if m.hosts != nil {
			context = hostName(s.Target)
		}
		values := []string{name, status, context, ""}
		parent := m.pickerDisplayRow(values)
		if len(rows) != cursor {
			if !s.Running {
				parent = styleTableRow(parent, stoppedSessionRowStyle)
			} else {
				parent[1] = successStyle.Render(parent[1])
			}
			parent[0] = titleStyle.Render(displayCell(name, columnWidth(m.table.Columns(), 0)))
			if context != "" {
				parent[2] = subtleStyle.Render(displayCell(context, columnWidth(m.table.Columns(), 2)))
			}
		}
		rows = append(rows, parent)
		if s.Running && s.Target == "" {
			for _, agent := range m.agents[s.ID()] {
				status := agent.State
				if status == "blocked" {
					status = "needs input"
				}
				values := []string{"  └ " + agentDisplayName(agent), status}
				cwd := agent.ForegroundCWD
				if strings.TrimSpace(cwd) == "" {
					cwd = agent.CWD
				}
				project := ""
				if strings.TrimSpace(cwd) != "" {
					project = filepath.Base(cwd)
				}
				title := agent.Title
				if strings.TrimSpace(title) == "" {
					title = agent.TerminalTitle
				}
				row := m.pickerDisplayRow(append(values, project, title))
				if len(rows) != cursor {
					row[0] = agentNameStyle.Render(row[0])
					row[1] = agentStatusStyle(agent.State).Render(row[1])
					if project != "" {
						row[2] = subtleStyle.Render(row[2])
					}
				}
				rows = append(rows, row)
			}
		}
	}
	return rows
}

func (m model) pickerDisplayRow(values []string) table.Row {
	row := make(table.Row, len(values))
	for i, value := range values {
		row[i] = displayCell(value, columnWidth(m.table.Columns(), i))
	}
	return row
}

func agentDisplayName(agent herdr.Agent) string {
	label := agent.DisplayName
	if strings.TrimSpace(label) == "" {
		label = agent.Kind
	}
	name := agent.Name
	if name == agent.Target || name == agent.PaneID {
		name = ""
	}
	if strings.TrimSpace(name) == "" {
		name = label
	}
	if strings.TrimSpace(name) == "" {
		return "Agent"
	}
	if label != "" && !strings.EqualFold(name, label) {
		return label + ": " + name
	}
	return name
}

func (m model) preferenceProgress() string {
	if m.preferencesPending {
		return "Saving or loading preferences…"
	}
	return ""
}
