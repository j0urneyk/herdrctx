package ui

import (
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
	s, ok := m.selectedSession()
	if !ok {
		m.showDialog(dialogWarning, "No session selected", "Select a session before changing favorites.")
		return m, nil
	}
	cmd := m.saveFavoriteCmd(preferences.Change{FavoriteName: s.Name, Favorite: !m.preferences.Favorite(s.Name)})
	return m, cmd
}

func (m model) navigationRows(sessions []herdr.Session, cursor int) []table.Row {
	rows := sessionRowsWithSelection(sessions, m.table.Columns(), cursor)
	for i, s := range sessions {
		if m.preferences.Favorite(s.Name) {
			rows[i][0] = displayCell("⭐\ufe0f "+s.DisplayName(), columnWidth(m.table.Columns(), 0))
			if !s.Running && i != cursor {
				rows[i][0] = stoppedSessionRowStyle.Render(rows[i][0])
			}
		}
	}
	return rows
}

func (m model) preferenceProgress() string {
	if m.preferencesPending {
		return "Saving or loading preferences…"
	}
	return ""
}
