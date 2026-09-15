package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/j0urneyk/herdrctx/internal/herdr"
)

func remoteModel() model {
	c := herdr.NewClient("herdr")
	c.Remote = &herdr.RemoteTarget{Destination: "workbox"}
	m := NewModel(Options{Client: c}).(model)
	m.loading = false
	m.setSessions([]herdr.Session{{Target: "workbox", Name: "api", Running: true}})
	return m
}

func TestRemoteStaleAttachChecksNewStatus(t *testing.T) {
	m := remoteModel()
	next, _ := m.Update(sessionsLoadedMsg{Err: errors.New("offline")})
	m = next.(model)
	if !m.remoteStale || !strings.Contains(m.navigationSummary(), "last known") {
		t.Fatal("stale state missing")
	}
	next, cmd := m.attachSelected()
	m = next.(model)
	if cmd == nil || m.remotePending == nil {
		t.Fatal("did not recheck")
	}
	next, _ = m.Update(remotePreflightMsg{ID: m.remotePending.ID, Sessions: []herdr.Session{{Target: "workbox", Name: "api"}}})
	m = next.(model)
	if m.dialog == nil || m.dialog.Title != "Stopped session attach disabled" || m.busy != "" {
		t.Fatalf("restarted stopped session: %+v", m.dialog)
	}
}

func TestRemotePreflightCancellationIgnoresLateResponse(t *testing.T) {
	m := remoteModel()
	m.remoteStale = true
	next, _ := m.confirmStop()
	m = next.(model)
	id := m.remotePending.ID
	m = navKey(m, "esc")
	next, cmd := m.Update(remotePreflightMsg{ID: id, Sessions: m.sessions})
	m = next.(model)
	if cmd != nil || m.confirm != nil || m.remotePending != nil || m.busy != "" {
		t.Fatal("cancelled check triggered an action")
	}
}

func TestRemoteConfirmationRechecksFailureAndTarget(t *testing.T) {
	for _, missing := range []bool{false, true} {
		m := remoteModel()
		next, _ := m.confirmStop()
		m = next.(model)
		if !strings.Contains(m.confirm.title(), "workbox") {
			t.Fatal("host omitted")
		}
		m.remoteStale = true
		next, _ = m.handleConfirmationKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(model)
		if m.remotePending == nil || !m.remotePending.Confirmed {
			t.Fatal("confirmation not rechecked")
		}
		msg := remotePreflightMsg{ID: m.remotePending.ID, Err: errors.New("offline")}
		if missing {
			msg.Err = nil
			msg.Sessions = []herdr.Session{{Target: "another-host", Name: "api", Running: true}}
		}
		next, _ = m.Update(msg)
		m = next.(model)
		if m.dialog == nil || m.busy != "" || m.confirm != nil {
			t.Fatal("invalid target executed")
		}
	}
}

func TestRemoteCreateAndNestedGuards(t *testing.T) {
	m := remoteModel()
	next, _ := m.openNewSession(newSessionWithDir)
	m = next.(model)
	if m.newSession != nil || m.dialog == nil || !strings.Contains(m.dialog.Title, "unavailable") {
		t.Fatal("remote N not blocked")
	}
	m.dialog = nil
	next, _ = m.openNewSession(newSessionQuick)
	m = next.(model)
	m.newSession.name.SetValue("api")
	m.newSession.defaultDir = "/does/not/exist"
	value, err := m.newSession.submit()
	if err != nil || value.Dir != "" {
		t.Fatalf("remote creation used local filesystem: %+v %v", value, err)
	}
	if !strings.Contains(m.newSession.render(100), "remote default directory") {
		t.Fatal("directory semantics missing")
	}
	m.newSession = nil
	m.insideHerdr = true
	next, _ = m.openNewSession(newSessionQuick)
	m = next.(model)
	if m.newSession != nil || m.dialog.Title != "Cannot create from inside Herdr" {
		t.Fatal("nested remote creation allowed")
	}
	m.dialog = nil
	next, _ = m.attachSelected()
	m = next.(model)
	if m.dialog == nil || m.dialog.Title != "Cannot attach from inside Herdr" {
		t.Fatal("nested remote attachment allowed")
	}
}

func TestRemoteUnknownOutcomeKeepsWarning(t *testing.T) {
	m := remoteModel()
	next, _ := m.Update(actionFinishedMsg{Action: confirmStop, Name: "api", Err: &herdr.OutcomeUnknownError{Target: "workbox", Action: "stop", Name: "api", Err: errors.New("lost response")}})
	m = next.(model)
	if !m.remoteStale || m.dialog == nil || m.dialog.Title != "Remote result unknown" {
		t.Fatal("outcome not distinguished")
	}
	if !strings.Contains(m.dialog.Body, "workbox") {
		t.Fatal("host missing")
	}
}

func TestRemotePreflightWaitsForBackgroundQuery(t *testing.T) {
	m := remoteModel()
	m.remoteStale = true
	_ = m.startRefresh()
	id := m.refreshRequestID
	next, cmd := m.attachSelected()
	m = next.(model)
	if cmd != nil || !m.loading || m.remotePending == nil {
		t.Fatal("started another query before the background query finished")
	}
	next, cmd = m.Update(sessionsLoadedMsg{RequestID: id, Sessions: m.sessions})
	m = next.(model)
	if cmd == nil || m.remotePending == nil || !m.remoteStale {
		t.Fatal("background response validated the action instead of starting a fresh check")
	}
	m.remotePending.Cancel()
}

func TestRemotePreflightCancellationWaitsForCommandExit(t *testing.T) {
	m := remoteModel()
	m.remoteStale = true
	next, _ := m.confirmStop()
	m = next.(model)
	firstID := m.remotePending.ID
	m = navKey(m, "esc")
	if !m.loading {
		t.Fatal("cancelled query was declared finished before command exit")
	}
	next, cmd := m.confirmStop()
	m = next.(model)
	if cmd != nil {
		t.Fatal("new query overlapped cancelled command")
	}
	secondID := m.remotePending.ID
	next, cmd = m.Update(remotePreflightMsg{ID: firstID, Sessions: m.sessions})
	m = next.(model)
	if cmd == nil || m.confirm != nil || m.remotePending == nil || m.remotePending.ID != secondID {
		t.Fatal("late response was reused or next query was not started")
	}
	next, _ = m.Update(remotePreflightMsg{ID: secondID, Sessions: m.sessions})
	m = next.(model)
	if m.confirm == nil || m.loading || m.remotePending != nil {
		t.Fatal("fresh check did not finish")
	}
}

func TestRemoteForegroundReturnQueuesRefresh(t *testing.T) {
	m := remoteModel()
	_ = m.startRefresh()
	id := m.refreshRequestID
	next, cmd := m.Update(attachFinishedMsg{Name: "api"})
	m = next.(model)
	if cmd != nil || !m.loading || !m.remoteRefreshQueued || m.refreshRequestID != id {
		t.Fatal("foreground return overlapped the running query")
	}
	next, cmd = m.Update(sessionsLoadedMsg{RequestID: id})
	m = next.(model)
	if cmd == nil || !m.loading || m.remoteRefreshQueued || m.refreshRequestID == id || len(m.sessions) != 1 {
		t.Fatal("queued refresh did not replace the old result")
	}
}

func TestRemoteFreshConfirmationWaitsForRunningRefresh(t *testing.T) {
	m := remoteModel()
	next, _ := m.confirmStop()
	m = next.(model)
	_ = m.startRefresh()
	next, cmd := m.handleConfirmationKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if cmd != nil || m.remotePending == nil || !m.remotePending.Waiting {
		t.Fatal("confirmation overlapped an active background refresh")
	}
	m.remotePending.Cancel()
}

func TestRemoteCancelledCheckAfterFailedRefreshRequiresRevalidation(t *testing.T) {
	for _, action := range []string{"attach", "stop", "delete"} {
		t.Run(action, func(t *testing.T) {
			m := remoteModel()
			if action == "delete" {
				m.sessions[0].Running = false
			}
			_ = m.startRefresh()
			refreshID := m.refreshRequestID
			next, _ := m.checkRemoteSession(m.sessions[0], action, false)
			m = next.(model)
			next, _ = m.Update(sessionsLoadedMsg{RequestID: refreshID, Err: errors.New("offline")})
			m = next.(model)
			checkID := m.remotePending.ID
			m = navKey(m, "esc")
			next, _ = m.Update(remotePreflightMsg{ID: checkID, Err: context.Canceled})
			m = next.(model)
			switch action {
			case "attach":
				next, _ = m.attachSelected()
			case "stop":
				next, _ = m.confirmStop()
			case "delete":
				next, _ = m.confirmDelete()
			}
			m = next.(model)
			if m.remotePending == nil || m.confirm != nil {
				t.Fatal("cancelled check authorized an action from the old list")
			}
			defer m.remotePending.Cancel()
			next, _ = m.Update(remotePreflightMsg{ID: m.remotePending.ID, Sessions: nil})
			m = next.(model)
			if m.dialog == nil || m.dialog.Title != "Session no longer available" || m.remoteStale || m.busy != "" {
				t.Fatal("fresh result did not replace the old session state")
			}
		})
	}
}
