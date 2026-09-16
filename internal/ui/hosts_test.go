package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/j0urneyk/herdrctx/internal/herdr"
	"github.com/j0urneyk/herdrctx/internal/preferences"
)

func multiModel() model {
	m := remoteModel()
	m.hostData = preferences.Hosts{Version: 1, Items: []preferences.Host{{Label: "One", Destination: "one"}, {Label: "Two", Destination: "two"}}}
	m.initHosts(true)
	for _, id := range m.hosts.order {
		h := m.hosts.entries[id]
		h.stale = false
		h.sessions = []herdr.Session{{Target: id, Name: "api", Running: true}}
	}
	m.mergeHostSessions()
	return m
}

func hostReply(m model, id string, err error) hostLoadedMsg {
	h := m.hosts.entries[id]
	return hostLoadedMsg{Target: id, Generation: h.generation, Request: h.request, Sessions: h.sessions, Err: err}
}

func TestHostSelectionSurvivesSameNameRefresh(t *testing.T) {
	m := multiModel()
	m.table.SetCursor(2)
	selected, _ := m.selectedSession()
	m.hosts.entries["one"].sessions = append(m.hosts.entries["one"].sessions, herdr.Session{Target: "one", Name: "aaa"})
	m.mergeHostSessions()
	got, _ := m.selectedSession()
	if got.ID() != selected.ID() {
		t.Fatalf("selection changed %v -> %v", selected.ID(), got.ID())
	}
	m.hosts.active = "two"
	m.configureTable()
	got, _ = m.selectedSession()
	if got.Target != "two" {
		t.Fatal(got)
	}
}

func TestHostQueryLimitAndFairness(t *testing.T) {
	m := multiModel()
	for i := 0; i < 10; i++ {
		m.addHost(preferences.Host{Label: fmt.Sprint(i), Destination: fmt.Sprintf("host%d", i)}, nil)
	}
	_ = m.queueHostRefresh()
	seen := map[string]bool{}
	for len(seen) < len(m.hosts.order) {
		active := 0
		id := ""
		for _, candidate := range m.hosts.order {
			if m.hosts.entries[candidate].loading {
				active++
				id = candidate
			}
		}
		if active > maxHostQueries || active == 0 {
			t.Fatalf("active=%d seen=%d", active, len(seen))
		}
		seen[id] = true
		next, _ := m.Update(hostReply(m, id, nil))
		m = next.(model)
	}
}

func TestHostCancellationAndGenerationIsolation(t *testing.T) {
	m := multiModel()
	m.hosts.active = "one"
	_ = m.queueHostRefresh()
	old := hostReply(m, "one", errors.New("offline"))
	next, _ := m.checkRemoteSession(m.hosts.entries["one"].sessions[0], "attach", false)
	m = next.(model)
	next, _ = m.Update(old)
	m = next.(model)
	pending := m.remotePending
	m = navKey(m, "esc")
	reply := hostReply(m, "one", context.Canceled)
	reply.CheckID = pending.ID
	next, _ = m.Update(reply)
	m = next.(model)
	if !m.hosts.entries["one"].stale || m.busy != "" {
		t.Fatal("cancellation lost stale state")
	}
	m.hosts.active = "two"
	m.configureTable()
	next, _ = m.Update(old)
	m = next.(model)
	s, _ := m.selectedSession()
	if s.Target != "two" {
		t.Fatal("late reply changed host")
	}
	old = hostReply(m, "one", nil)
	delete(m.hosts.entries, "one")
	m.addHost(preferences.Host{Label: "Replacement", Destination: "one"}, nil)
	next, _ = m.Update(old)
	m = next.(model)
	if len(m.hosts.entries["one"].sessions) != 0 {
		t.Fatal("previous generation accepted")
	}
}

func TestHostModalAndCreationTarget(t *testing.T) {
	m := multiModel()
	next, _ := m.openNewSession(newSessionQuick)
	m = next.(model)
	if m.hostMenu == nil || !m.hostMenu.create || m.newSession != nil {
		t.Fatal("aggregate creation guessed target")
	}
	m = navKey(m, "A")
	if m.hostMenu == nil {
		t.Fatal("creation accepted all hosts")
	}
	m.hostMenu.cursor = 1
	m = navKey(m, "enter")
	if m.newSession == nil || m.newSession.remoteTarget != "one" {
		t.Fatal("creation target missing")
	}
	m = navKey(m, "H")
	if m.hostMenu != nil {
		t.Fatal("form leaked switch")
	}
	m = navKey(m, "esc")
	m.hosts.active = "two"
	m.configureTable()
	next, _ = m.confirmStop()
	m = next.(model)
	m = navKey(m, "H")
	if m.confirm == nil || m.hostMenu != nil {
		t.Fatal("confirmation leaked switch")
	}
}

func TestHostQueryFailureIsPartial(t *testing.T) {
	m := multiModel()
	_ = m.queueHostRefresh()
	next, _ := m.Update(hostReply(m, "one", errors.New("offline")))
	m = next.(model)
	if !m.hosts.entries["one"].stale || m.hosts.entries["two"].stale || len(m.hosts.entries["one"].sessions) != 1 {
		t.Fatal("failure affected another host or removed cached list")
	}
}

func TestHostActionUsesCapturedClient(t *testing.T) {
	m := multiModel()
	for _, id := range []string{"one", "two"} {
		c := m.clientFor(id)
		cmd, err := c.AttachCommand("api")
		if err != nil || cmd.Args[2] != id {
			t.Fatalf("%v %v", cmd, err)
		}
	}
	m.hosts.active = "two"
	c := m.clientFor("one")
	m.hosts.active = ""
	if c.Remote.Destination != "one" {
		t.Fatal("client mutated")
	}
}

func TestMachineImportRequiresConflictConfirmation(t *testing.T) {
	m := multiModel()
	m.hostMenu = &hostMenu{}
	m.machineMenu = &machineMenu{rows: []herdr.Machine{{ID: "profile", Label: "New", Target: "one", Session: "api", Enabled: true}}}
	next := handleMachinesInput(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.model
	if m.machineMenu == nil || m.machineMenu.conflict == "" || next.cmd != nil {
		t.Fatal("conflict replaced without confirmation")
	}
}

func TestCancelledMachineConflictDoesNotReplaceAnotherHost(t *testing.T) {
	m := multiModel()
	m.hostStore = &preferences.HostStore{Path: filepath.Join(t.TempDir(), "hosts.json")}
	for _, h := range m.hostData.Items {
		if _, err := m.hostStore.Apply(preferences.HostChange{Host: h}); err != nil {
			t.Fatal(err)
		}
	}
	m.hostMenu = &hostMenu{}
	m.machineMenu = &machineMenu{rows: []herdr.Machine{{ID: "existing", Label: "Replace", Target: "one", Session: "api", Enabled: true}, {ID: "new", Label: "New", Target: "three", Session: "api", Enabled: true}}}
	m = handleMachinesInput(m, tea.KeyPressMsg{Code: tea.KeyEnter}).model
	m = handleMachinesInput(m, tea.KeyPressMsg{Code: tea.KeyEscape}).model
	m = handleMachinesInput(m, tea.KeyPressMsg{Code: tea.KeyDown}).model
	result := handleMachinesInput(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if result.cmd == nil {
		t.Fatal("new profile did not save")
	}
	reply := result.cmd().(hostsSavedMsg)
	if reply.Err != nil || len(reply.Data.Items) != 3 {
		t.Fatalf("cancelled replacement applied: %+v %v", reply.Data, reply.Err)
	}
}

func TestHostMenusKeepSelectedRowVisible(t *testing.T) {
	m := multiModel()
	m.height = 20
	for i := 0; i < 30; i++ {
		m.addHost(preferences.Host{Label: fmt.Sprintf("Label%d", i), Destination: fmt.Sprintf("newhost%d", i)}, nil)
	}
	m.hostMenu = &hostMenu{cursor: len(m.hosts.order) - 1}
	view := m.hostMenuView()
	if strings.Contains(view, "Label0 ·") || !strings.Contains(view, "newhost29") {
		t.Fatal("host menu did not follow selection")
	}
	m.machineMenu = &machineMenu{cursor: 29}
	for i := 0; i < 30; i++ {
		m.machineMenu.rows = append(m.machineMenu.rows, herdr.Machine{ID: fmt.Sprint(i), Label: fmt.Sprintf("Profile%d", i), Target: "host", Session: "api", Enabled: true})
	}
	view = m.machineMenuView()
	if strings.Contains(view, "Profile0 ·") || !strings.Contains(view, "Profile29") {
		t.Fatal("machine menu did not follow selection")
	}
}

func TestQueuedHostActionWaitsForQuerySlot(t *testing.T) {
	m := multiModel()
	m.addHost(preferences.Host{Label: "Last", Destination: "last"}, nil)
	last := m.hosts.entries["last"]
	last.stale = false
	last.sessions = []herdr.Session{{Target: "last", Name: "api", Running: true}}
	m.mergeHostSessions()
	_ = m.queueHostRefresh()
	if last.loading || !last.queued {
		t.Fatal("last host was not queued")
	}
	next, cmd := m.attachSession(last.sessions[0])
	m = next.(model)
	if m.remotePending == nil || !m.remotePending.Waiting || cmd != nil {
		t.Fatal("queued refresh was bypassed by attachment")
	}
	next, cmd = m.Update(hostReply(m, "one", nil))
	m = next.(model)
	if cmd == nil || m.remotePending == nil || m.remotePending.Waiting || !last.loading {
		t.Fatal("check did not start after slot became available")
	}
	m.remotePending.Cancel()
}

func TestInvalidHostCatalogDoesNotEnableTargets(t *testing.T) {
	m := remoteModel()
	m.hostError = errors.New("invalid hosts file")
	m.hostData = preferences.Hosts{Version: 1, Items: []preferences.Host{{Label: "Invalid", Destination: "invalid-host"}}}
	next, _ := m.openHosts(false, newSessionQuick)
	m = next.(model)
	if m.hostMenu == nil || m.hostMenu.err == "" || m.hosts.entries["invalid-host"] != nil {
		t.Fatal("invalid host catalog was silently accepted")
	}
}

func TestDeletePreflightWaitsForGlobalQuerySlot(t *testing.T) {
	m := multiModel()
	m.addHost(preferences.Host{Label: "Fifth", Destination: "fifth"}, nil)
	m.hosts.entries[""].sessions[0].Running = false
	_ = m.queueHostRefresh()
	next, _ := m.Update(hostReply(m, "", nil))
	m = next.(model)
	// Local is fresh; all four remote hosts now occupy the query slots.
	m.confirm = &confirmation{Action: confirmDelete, Session: m.hosts.entries[""].sessions[0]}
	next, cmd := m.handleConfirmationKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if cmd != nil || m.remotePending == nil || !m.remotePending.Waiting {
		t.Fatal("delete preflight exceeded the global query limit")
	}
	m.remotePending.Cancel()
}
