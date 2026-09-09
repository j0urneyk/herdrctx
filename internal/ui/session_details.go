package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/j0urneyk/herdrctx/internal/herdr"
)

type sessionDetails struct {
	session  herdr.Session
	notice   string
	viewport viewport.Model
}

func (m model) openDetails() model {
	s, ok := m.selectedSession()
	if !ok {
		m.showDialog(dialogWarning, "No session selected", "Select a session to view its details.")
		return m
	}
	m.details = &sessionDetails{session: s, viewport: viewport.New()}
	m.resizeNavigation()
	return m
}

func (m *model) updateDetails() {
	if m.details == nil {
		return
	}
	if s, ok := m.sessionByName(m.details.session.Name); ok {
		m.details.session = s
		m.details.notice = ""
	} else {
		m.details.notice = "Session no longer listed"
	}
	m.resizeNavigation()
}

func navigationWidth(width int) int {
	if width <= 0 {
		width = 100
	}
	return max(12, min(84, width-6))
}

func (m *model) resizeNavigation() {
	if m.details != nil {
		d := m.details
		width := navigationWidth(m.width)
		d.viewport.SetWidth(max(1, width-4))
		d.viewport.SetHeight(max(1, min(18, m.height-10)))
		field := func(s string) string {
			if s == "" {
				return "Not available"
			}
			return sanitizeDisplay(s)
		}
		body := fmt.Sprintf("Name: %s\nStatus: %s\nDefault session: %t\nFavorite: %t\n\nSession state directory:\n%s\n\nSocket path:\n%s", field(d.session.Name), d.session.Status(), d.session.Default, m.preferences.Favorite(d.session.Name), field(d.session.SessionDir), field(d.session.SocketPath))
		d.viewport.SetContent(lipgloss.Wrap(body, max(1, width-4), ""))
	}
}

func (d sessionDetails) render(width int) string {
	parts := []string{titleStyle.Render("Session details")}
	if d.notice != "" {
		parts = append(parts, warningStyle.Render(d.notice))
	}
	parts = append(parts, d.viewport.View(), "↑/↓ PgUp/PgDn scroll · Enter/Esc/q closes")
	return dialogBoxStyle.Padding(0, 1).Width(navigationWidth(width)).Render(strings.Join(parts, "\n"))
}

func handleDetailsInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	switch msg.String() {
	case "enter", "esc", "q":
		m.details = nil
		return inputConsumed(m, nil)
	}
	var cmd tea.Cmd
	m.details.viewport, cmd = m.details.viewport.Update(msg)
	return inputConsumed(m, cmd)
}
