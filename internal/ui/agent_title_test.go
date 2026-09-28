package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/j0urneyk/herdrctx/internal/herdr"
)

func TestAgentRenamingTitleAnimatesBetweenRefreshes(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.loading = false
	m.width, m.height = 160, 40
	session := herdr.Session{Name: "work", Running: true}
	m.setSessions([]herdr.Session{session})
	next, cmd := m.Update(agentsLoadedMsg{ID: session.ID(), Agents: []herdr.Agent{{Target: "w1:p1", State: "working", Title: "renaming... ⠚ | herdrctx"}}})
	m = next.(model)
	before := ansi.Strip(m.table.Rows()[1][3])
	if cmd == nil {
		t.Fatalf("renaming title has no animation scheduled: %q", before)
	}
	for i := 0; i < 3; i++ {
		if cmd == nil {
			t.Fatal("renaming animation stopped before title completion")
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			if len(batch) != 1 {
				t.Fatalf("unexpected commands during title animation: %d", len(batch))
			}
			msg = batch[0]()
		}
		next, cmd = m.Update(msg)
		m = next.(model)
		after := ansi.Strip(m.table.Rows()[1][3])
		if before == after {
			t.Fatalf("renaming title stayed frozen: %q", after)
		}
		frame, text, ok := strings.Cut(after, " ")
		runes := []rune(frame)
		if !ok || len(runes) != 1 || runes[0] < '\u2800' || runes[0] > '\u28ff' || text != "renaming... | herdrctx" {
			t.Fatalf("renaming spinner must precede the title text: %q", after)
		}
		before = after
	}
}

func TestAgentRenamingAnimationStopsAndRejectsOldTicks(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.loading = false
	m.width = 160
	session := herdr.Session{Name: "work", Running: true}
	m.setSessions([]herdr.Session{session})
	pending := agentsLoadedMsg{ID: session.ID(), Agents: []herdr.Agent{{Target: "w1:p1", TerminalTitle: "renaming... ⠚ | herdrctx"}}}
	next, cmd := m.Update(pending)
	m = next.(model)
	if cmd == nil {
		t.Fatal("fallback terminal title did not animate")
	}
	oldTick := cmd()
	m.table.SetCursor(1)
	next, cmd = m.Update(agentsLoadedMsg{ID: session.ID(), Agents: []herdr.Agent{{Target: "w1:p1", Title: "Fix session picker", TerminalTitle: "renaming... ⠚ | herdrctx"}}})
	m = next.(model)
	if cmd != nil || m.agentTitleAnimating || ansi.Strip(m.table.Rows()[1][3]) != "Fix session picker" {
		t.Fatal("completed title did not stop animation")
	}
	next, cmd = m.Update(oldTick)
	m = next.(model)
	if cmd != nil || m.table.Cursor() != 1 || ansi.Strip(m.table.Rows()[1][3]) != "Fix session picker" {
		t.Fatal("late animation tick changed the completed title or selection")
	}
	next, cmd = m.Update(pending)
	m = next.(model)
	if cmd == nil {
		t.Fatal("second renaming did not restart animation")
	}
	before := m.table.Rows()[1][3]
	next, staleCmd := m.Update(oldTick)
	m = next.(model)
	if staleCmd != nil || m.table.Rows()[1][3] != before {
		t.Fatal("old animation tick affected the new animation")
	}
	next, cmd = m.Update(cmd())
	m = next.(model)
	if cmd == nil || m.table.Rows()[1][3] == before || m.table.Cursor() != 1 {
		t.Fatal("new animation failed to advance while keeping selection")
	}
	if strings.Contains(strings.Join(m.table.Rows()[1], ""), "\x1b") {
		t.Fatal("title animation inserted colors into the selected row")
	}
}

func TestAgentRenamingAnimationFollowsVisibleRows(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.loading = false
	session := herdr.Session{Name: "work", Running: true}
	m.setSessions([]herdr.Session{session})
	next, cmd := m.Update(agentsLoadedMsg{ID: session.ID(), Agents: []herdr.Agent{{Target: "w1:p1", Title: "renaming... ⠚ | herdrctx"}}})
	m = next.(model)
	if cmd == nil {
		t.Fatal("animation did not start")
	}
	oldTick := cmd()
	key := tea.KeyPressMsg{Code: 'f', Text: "f"}
	next, _ = m.Update(key)
	next, cmd = next.(model).Update(key)
	m = next.(model)
	if m.agentTitleAnimating || cmd != nil || len(m.table.Rows()) != 0 {
		t.Fatal("filtered-out title kept animating")
	}
	next, cmd = m.Update(oldTick)
	m = next.(model)
	if cmd != nil {
		t.Fatal("hidden animation queued another tick")
	}
	next, cmd = m.Update(key)
	m = next.(model)
	if !m.agentTitleAnimating || cmd == nil {
		t.Fatal("visible title did not resume animation")
	}
	next, cmd = m.Update(agentsLoadedMsg{ID: session.ID()})
	m = next.(model)
	if m.agentTitleAnimating || cmd != nil {
		t.Fatal("removed agent kept animating")
	}
}

func TestAgentTitlesPreserveOrdinaryText(t *testing.T) {
	m := NewModel(Options{}).(model)
	for _, title := range []string{"Review changes", "renaming...", "renaming... notes | project", "Discuss renaming... ⠚ | project", "renaming... ⠚ a symbol"} {
		if got := m.agentTitleView(herdr.Agent{Title: title}); got != title {
			t.Fatalf("ordinary title changed: %q -> %q", title, got)
		}
	}
}
