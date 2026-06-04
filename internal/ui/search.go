package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/j0urneyk/herdrctx/internal/herdr"
)

type sessionSearchScope int

const (
	sessionSearchScopeName sessionSearchScope = iota
	sessionSearchScopeDirectory
)

type sessionSearch struct {
	active bool
	scope  sessionSearchScope
	input  textinput.Model
}

func newSessionSearch(width int) sessionSearch {
	input := textinput.New()
	search := sessionSearch{
		scope: sessionSearchScopeName,
		input: input,
	}
	search.setWidth(width)
	search.updatePrompt()

	return search
}

func (s sessionSearchScope) String() string {
	switch s {
	case sessionSearchScopeDirectory:
		return "directory"
	default:
		return "name"
	}
}

func (s sessionSearchScope) next() sessionSearchScope {
	if s == sessionSearchScopeDirectory {
		return sessionSearchScopeName
	}

	return sessionSearchScopeDirectory
}

func (s *sessionSearch) setWidth(width int) {
	if width <= 0 {
		width = 100
	}
	s.input.SetWidth(max(20, min(80, width-32)))
}

func (s *sessionSearch) open(width int) tea.Cmd {
	s.active = true
	s.setWidth(width)
	s.updatePrompt()
	s.input.CursorEnd()

	return s.input.Focus()
}

func (s *sessionSearch) close() {
	s.active = false
	s.input.Blur()
}

func (s *sessionSearch) clear() {
	s.input.SetValue("")
	s.input.CursorEnd()
}

func (s *sessionSearch) toggleScope() {
	s.scope = s.scope.next()
	s.updatePrompt()
	s.input.CursorEnd()
}

func (s *sessionSearch) updatePrompt() {
	s.input.Prompt = "Search " + s.scope.String() + ": "
	if s.scope == sessionSearchScopeDirectory {
		s.input.Placeholder = "directory path"
		return
	}

	s.input.Placeholder = "session name"
}

func (s sessionSearch) query() string {
	return s.input.Value()
}

func (s sessionSearch) hasQuery() bool {
	return s.query() != ""
}

func (s sessionSearch) view() string {
	input := s.input
	input.SetValue(sanitizeDisplay(input.Value()))
	input.Placeholder = sanitizeDisplay(input.Placeholder)

	return input.View()
}

func filterSessionsBySearch(sessions []herdr.Session, search sessionSearch) []herdr.Session {
	if !search.hasQuery() {
		return sessions
	}

	filtered := make([]herdr.Session, 0, len(sessions))
	for _, session := range sessions {
		if sessionMatchesSearch(session, search) {
			filtered = append(filtered, session)
		}
	}

	return filtered
}

func sessionMatchesSearch(session herdr.Session, search sessionSearch) bool {
	query := search.query()
	if query == "" {
		return true
	}

	value := session.Name
	if search.scope == sessionSearchScopeDirectory {
		value = session.SessionDir
	}

	return containsFold(value, query)
}

func containsFold(value string, query string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(query))
}
