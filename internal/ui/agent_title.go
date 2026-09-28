package ui

import (
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/j0urneyk/herdrctx/internal/herdr"
)

func agentReportedTitle(agent herdr.Agent) string {
	if strings.TrimSpace(agent.Title) != "" {
		return agent.Title
	}
	return agent.TerminalTitle
}

// Only replace the sampled spinner in the reported temporary renaming title.
func agentRenamingTitleSuffix(title string) (string, bool) {
	rest, ok := strings.CutPrefix(title, "renaming... ")
	if !ok {
		return "", false
	}
	frame, size := utf8.DecodeRuneInString(rest)
	if frame < '\u2800' || frame > '\u28ff' {
		return "", false
	}
	suffix := rest[size:]
	if suffix != "" && !strings.HasPrefix(suffix, " | ") {
		return "", false
	}
	return suffix, true
}

func (m model) agentTitleView(agent herdr.Agent) string {
	title := agentReportedTitle(agent)
	if suffix, ok := agentRenamingTitleSuffix(title); ok {
		return m.agentTitleSpinner.View() + " renaming..." + suffix
	}
	return title
}

func (m model) hasRenamingAgentTitle() bool {
	for _, session := range m.visibleSessions() {
		if !session.Running || session.Target != "" {
			continue
		}
		for _, agent := range m.agents[session.ID()] {
			if _, ok := agentRenamingTitleSuffix(agentReportedTitle(agent)); ok {
				return true
			}
		}
	}
	return false
}

func (m *model) syncAgentTitleAnimation() tea.Cmd {
	if m.busy != "" || !m.hasRenamingAgentTitle() {
		m.agentTitleAnimating = false
		return nil
	}
	if m.agentTitleAnimating {
		return nil
	}
	// A new spinner ID rejects any queued tick from the previous animation.
	m.agentTitleSpinner = spinner.New(spinner.WithSpinner(spinner.MiniDot))
	m.agentTitleAnimating = true
	m.refreshTableRowsForCurrentCursor()
	return m.agentTitleSpinner.Tick
}
