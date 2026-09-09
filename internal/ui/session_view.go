package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/j0urneyk/herdrctx/internal/herdr"
)

type statusFilter int

const (
	filterAll statusFilter = iota
	filterRunning
	filterStopped
)

func (f statusFilter) String() string { return [...]string{"All", "Running", "Stopped"}[f] }
func (m model) orderedSessions() []herdr.Session {
	visible := make([]herdr.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		if m.filter == filterRunning && !s.Running || m.filter == filterStopped && s.Running {
			continue
		}
		if sessionMatchesSearch(s, m.search) {
			visible = append(visible, s)
		}
	}
	slices.SortFunc(visible, func(a, b herdr.Session) int {
		af, bf := m.preferences.Favorite(a.Name), m.preferences.Favorite(b.Name)
		if af != bf {
			if af {
				return -1
			}
			return 1
		}
		if m.runningFirst && a.Running != b.Running {
			if a.Running {
				return -1
			}
			return 1
		}
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return visible
}

func (m model) navigationSummary() string {
	order := "Name"
	if m.runningFirst {
		order = "Running first"
	}
	return fmt.Sprintf("Status: %s · Sort: %s · %d/%d sessions", m.filter, order, m.visibleSessionCount(), len(m.sessions))
}
