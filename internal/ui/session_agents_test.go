package ui

import (
	"slices"
	"testing"

	"github.com/j0urneyk/herdrctx/internal/herdr"
)

func TestAgentAttachRevalidatesStaleHost(t *testing.T) {
	m := multiModel()
	session := m.hosts.entries[""].sessions[0]
	m.agents[session.ID()] = []herdr.Agent{{Target: "w1:p2", State: "working"}}
	m.hosts.entries[""].stale = true
	m.configureTable()
	m.table.SetCursor(1)
	m = navKey(m, "enter")
	if m.remotePending == nil || m.remotePending.Session != session.ID() {
		t.Fatal("agent attach bypassed the stale host check")
	}
	reply := hostReply(m, "", nil)
	reply.CheckID = m.remotePending.ID
	reply.Sessions = []herdr.Session{{Name: session.Name}}
	next, _ := m.Update(reply)
	m = next.(model)
	if m.dialog == nil || m.dialog.Title != "Session stopped" || m.busy != "" {
		t.Fatalf("stopped parent was attached: dialog=%#v busy=%q", m.dialog, m.busy)
	}
}

func TestAgentRefreshRejectsPreviousSessionLifetime(t *testing.T) {
	for _, removed := range []bool{false, true} {
		name := "stopped"
		if removed {
			name = "deleted"
		}
		t.Run(name, func(t *testing.T) {
			m := NewModel(Options{}).(model)
			session := herdr.Session{Name: "work", Running: true}
			m.setSessions([]herdr.Session{session})
			if len(m.loadAllAgentsCmds()) != 1 {
				t.Fatal("no initial query")
			}
			if removed {
				m.setSessions(nil)
			} else {
				m.setSessions([]herdr.Session{{Name: "work"}})
			}
			m.setSessions([]herdr.Session{session})
			if len(m.loadAllAgentsCmds()) != 0 {
				t.Fatal("overlapping agent query")
			}
			next, cmd := m.Update(agentsLoadedMsg{ID: session.ID(), Agents: []herdr.Agent{{Target: "w1:p2", Name: "old-process"}}})
			m = next.(model)
			if len(m.agents[session.ID()]) != 0 {
				t.Fatal("replacement session accepted previous lifetime's agents")
			}
			if _, loading := m.agentLoading[session.ID()]; cmd == nil || !loading {
				t.Fatal("replacement session did not start a fresh query after old command exited")
			}
			next, _ = m.Update(agentsLoadedMsg{ID: session.ID(), Agents: []herdr.Agent{{Target: "w2:p1", Name: "new-process"}}})
			m = next.(model)
			if agents := m.agents[session.ID()]; len(agents) != 1 || agents[0].Name != "new-process" {
				t.Fatalf("fresh replacement agents = %#v", agents)
			}
		})
	}
}

func TestAgentDetailsBlockedDuringFavoriteSave(t *testing.T) {
	m := NewModel(Options{}).(model)
	session := herdr.Session{Name: "work", Running: true}
	m.setSessions([]herdr.Session{session})
	m.agents[session.ID()] = []herdr.Agent{{Target: "w1:p2"}}
	m.configureTable()
	m.preferencesPending = true
	m = navKey(m, "down")
	m = navKey(m, "i")
	if m.details != nil || m.dialog == nil || m.dialog.Title != "Select a session" {
		t.Fatal("preferences input layer opened session details from agent row")
	}
}

func TestAgentAttachPreservesTargetThroughHostCheck(t *testing.T) {
	for _, state := range []string{"stale", "loading", "queued"} {
		t.Run(state, func(t *testing.T) {
			m := multiModel()
			h := m.hosts.entries[""]
			session := h.sessions[0]
			agent := herdr.Agent{Target: "w1:p2"}
			h.stale = state == "stale"
			h.loading = state == "loading"
			h.queued = state == "queued"
			next, _ := m.attachAgent(session, agent)
			m = next.(model)
			if m.remotePending == nil || m.remotePending.AgentTarget != agent.Target || m.remotePending.Session != session.ID() {
				t.Fatalf("lost agent identity: %#v", m.remotePending)
			}
			if state == "loading" {
				if !m.remotePending.Waiting {
					t.Fatal("did not wait for earlier host query")
				}
				next, _ = m.Update(hostReply(m, "", nil))
				m = next.(model)
				if m.remotePending == nil || m.remotePending.Waiting {
					t.Fatal("did not start fresh preflight")
				}
			}
			reply := hostReply(m, "", nil)
			reply.CheckID = m.remotePending.ID
			next, cmd := m.Update(reply)
			m = next.(model)
			if m.remotePending != nil || m.dialog != nil || m.busy != "attach agent in \"api\"" || cmd == nil {
				t.Fatalf("agent handoff lost after check: pending=%#v dialog=%#v busy=%q", m.remotePending, m.dialog, m.busy)
			}
		})
	}
}

func TestAgentAttachHostCheckCancellationAndMissingParent(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		m := multiModel()
		h := m.hosts.entries[""]
		h.stale = true
		next, _ := m.attachAgent(h.sessions[0], herdr.Agent{Target: "w1:p2"})
		m = next.(model)
		reply := hostReply(m, "", nil)
		reply.CheckID = m.remotePending.ID
		reply.Sessions = nil
		if cancel {
			m = navKey(m, "esc")
		}
		next, _ = m.Update(reply)
		m = next.(model)
		if m.busy != "" || m.remotePending != nil {
			t.Fatal("obsolete agent attach remained active")
		}
		if !cancel && (m.dialog == nil || m.dialog.Title != "Session no longer available") {
			t.Fatal("missing parent was not reported")
		}
		if cancel && m.dialog != nil {
			t.Fatal("cancelled check opened a dialog")
		}
	}
}

func TestAgentFocusCompletesBeforeCapturedSessionAttach(t *testing.T) {
	for _, fail := range []bool{false, true} {
		body := `
if [ "$1" = "--session" ] && [ "$2" = "work" ] && [ "$3" = "agent" ] && [ "$4" = "focus" ] && [ "$5" = "w1:p2" ]; then
  exit 0
fi
exit 2
`
		if fail {
			body = `echo 'selected agent disappeared' >&2; exit 1`
		}
		m := NewModel(Options{Client: herdr.NewClient(fakeHerdrCommand(t, body))}).(model)
		session := herdr.Session{Name: "work", Running: true}
		m.setSessions([]herdr.Session{session, {Name: "other", Running: true}})
		next, cmd := m.attachAgent(session, herdr.Agent{Target: "w1:p2"})
		m = next.(model)
		if cmd == nil || m.busy == "" {
			t.Fatal("agent selection did not enter busy focus phase")
		}
		m.table.SetCursor(0)
		msg, ok := cmd().(agentFocusFinishedMsg)
		if !ok || msg.Session.ID() != session.ID() || !slices.Equal(msg.Command.Args[1:], []string{"session", "attach", "work"}) {
			t.Fatalf("focus lost captured session: %#v", msg)
		}
		next, cmd = m.Update(msg)
		m = next.(model)
		if fail {
			if m.dialog == nil || m.dialog.Title != "Agent focus failed" || m.busy != "" {
				t.Fatal("failed focus did not stop attachment")
			}
		} else if msg.Err != nil || m.dialog != nil || cmd == nil || m.busy == "" {
			t.Fatal("successful focus did not proceed to captured session attachment")
		}
	}
}
