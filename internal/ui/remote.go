package ui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/j0urneyk/herdrctx/internal/herdr"
)

type remotePreflight struct {
	ID        uint64
	Session   herdr.SessionID
	Action    string
	Confirmed bool
	Waiting   bool
	Context   context.Context
	Cancel    context.CancelFunc
}
type remotePreflightMsg struct {
	ID       uint64
	Sessions []herdr.Session
	Err      error
}

func (m model) needsRemoteCheck() bool { return m.client.Remote != nil && (m.remoteStale || m.loading) }

func (m model) checkRemoteSession(s herdr.Session, action string, confirmed bool) (tea.Model, tea.Cmd) {
	m.remoteRequestID++
	ctx, cancel := context.WithCancel(m.ctx)
	pending := &remotePreflight{ID: m.remoteRequestID, Session: s.ID(), Action: action, Confirmed: confirmed, Cancel: cancel, Context: ctx, Waiting: m.loading}
	m.remotePending = pending
	m.showRefreshing = false
	m.busy = "checking remote session"
	m.setStatus("Checking remote session… Esc cancels.", statusInfo)
	if pending.Waiting {
		return m, nil
	}
	return m.startRemotePreflight()
}

func (m model) startRemotePreflight() (tea.Model, tea.Cmd) {
	pending := m.remotePending
	pending.Waiting = false
	// Wait for the previous command to exit, then fetch new data for this action.
	m.refreshRequestID++
	m.loading = true
	// A cancelled check must not make a discarded background result look fresh.
	m.remoteStale = true
	m.remoteRefreshQueued = false
	m.remoteCheckingID = pending.ID
	return m, func() tea.Msg {
		defer pending.Cancel()
		sessions, err := m.client.ListSessions(pending.Context)
		return remotePreflightMsg{ID: pending.ID, Sessions: sessions, Err: err}
	}
}

func (m model) handleRemotePreflight(msg remotePreflightMsg) (tea.Model, tea.Cmd) {
	if m.remoteCheckingID == 0 || msg.ID != m.remoteCheckingID {
		return m, nil
	}
	m.remoteCheckingID = 0
	m.loading = false
	if m.remotePending != nil && m.remotePending.Waiting {
		return m.startRemotePreflight()
	}
	if m.remoteRefreshQueued {
		cmd := m.reloadAfterAction()
		return m, cmd
	}
	if m.remotePending == nil || m.remotePending.ID != msg.ID {
		return m, nil
	}
	pending := *m.remotePending
	m.remotePending = nil
	m.busy = ""
	if msg.Err != nil {
		m.remoteStale = true
		m.showDialog(dialogError, "Remote check failed", fmt.Sprintf("No action was run on %s.\n\n%v", pending.Session.Target, msg.Err))
		return m, nil
	}
	m.remoteStale = false
	m.lastRefresh = time.Now()
	m.setSessions(msg.Sessions)
	m.updateDetails()
	s, ok := m.sessionByID(pending.Session)
	if !ok {
		m.showDialog(dialogWarning, "Session no longer available", "The selected session is no longer listed on this host.")
		return m, nil
	}
	if pending.Action == "attach" {
		return m.attachSession(s)
	}
	confirmed := confirmation{Action: confirmAction(pending.Action), Session: s}
	if !m.revalidateConfirmation(&confirmed) {
		return m, nil
	}
	if pending.Confirmed {
		return m.executeConfirmation(confirmed)
	}
	m.confirm = &confirmed
	return m, nil
}

func (m *model) reloadAfterAction() tea.Cmd {
	if m.client.Remote != nil && m.loading {
		m.remoteRefreshQueued = true
		return nil
	}
	m.remoteRefreshQueued = false
	return tea.Batch(m.startRefresh(), m.loadSessionsCmd(), m.spinner.Tick)
}

func (m model) sessionByID(id herdr.SessionID) (herdr.Session, bool) {
	for _, s := range m.sessions {
		if s.ID() == id {
			return s, true
		}
	}
	return herdr.Session{}, false
}

func handleRemotePreflightInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	if msg.String() == "esc" {
		m.remotePending.Cancel()
		m.remotePending = nil
		m.busy = ""
		m.setStatus("Remote check cancelled. No action was run.", statusInfo)
	}
	return inputConsumed(m, nil)
}
