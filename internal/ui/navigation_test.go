package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/j0urneyk/herdrctx/internal/herdr"
	"github.com/j0urneyk/herdrctx/internal/preferences"
)

func navKey(m model, key string) model {
	code := rune(0)
	switch key {
	case "enter":
		code = tea.KeyEnter
	case "esc":
		code = tea.KeyEscape
	case "down":
		code = tea.KeyDown
	default:
		code = []rune(key)[0]
	}
	next, _ := m.handleKey(tea.KeyPressMsg{Code: code, Text: func() string {
		if len(key) == 1 {
			return key
		}
		return ""
	}()})
	return next.(model)
}

func TestNavigationFiltersSortFavoritesAndSelection(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.loading = false
	original := []herdr.Session{{Name: "web", Running: true}, {Name: "api"}, {Name: "API", Running: true}}
	m.setSessions(original)
	assertSessionNames(t, m.visibleSessions(), []string{"API", "api", "web"})
	m = navKey(m, "f")
	assertSessionNames(t, m.visibleSessions(), []string{"API", "web"})
	m.preferences.Favorites = []string{"web"}
	m.configureTable()
	assertSessionNames(t, m.visibleSessions(), []string{"web", "API"})
	m.search.input.SetValue("api")
	m.configureTable()
	assertSessionNames(t, m.visibleSessions(), []string{"API"})
	if original[0].Name != "web" {
		t.Fatal("mutated source")
	}
	m = navKey(m, "i")
	m = navKey(m, "f")
	if m.filter != filterRunning {
		t.Fatal("details leaked input")
	}
	next, _ := m.Update(sessionsLoadedMsg{Sessions: []herdr.Session{{Name: "web"}}})
	m = next.(model)
	if m.details.notice != "Session no longer listed" {
		t.Fatal(m.details.notice)
	}
	next, _ = m.Update(sessionsLoadedMsg{Err: errors.New("offline")})
	m = next.(model)
	if !strings.Contains(m.details.render(60), "last known") {
		t.Fatal("missing refresh warning")
	}
	m = navKey(m, "q")
	if m.details != nil {
		t.Fatal("details remained open")
	}
}

func TestAgentRowsNestAndExposeBlockedNeedsInput(t *testing.T) {
	m := NewModel(Options{}).(model)
	session := herdr.Session{Name: "work", Running: true}
	m.setSessions([]herdr.Session{session})
	m.agents[session.ID()] = []herdr.Agent{{Name: "review", Kind: "codex", Target: "w1:p2", State: "blocked", WorkspaceID: "w1", PaneID: "w1:p2"}}
	m.configureTable()
	rows := m.table.Rows()
	if len(rows) != 2 || !strings.Contains(rows[0][0], "work") || !strings.Contains(rows[1][0], "review") || !strings.Contains(rows[1][1], "needs in") {
		t.Fatalf("picker rows = %#v", rows)
	}
	m.table.SetCursor(1)
	s, agent, ok := m.selectedAgent()
	if !ok || s.Name != "work" || agent.Target != "w1:p2" {
		t.Fatalf("selection = %#v %#v %v", s, agent, ok)
	}
}

func TestStoppedSessionDoesNotQueueAgentInspection(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.sessions = []herdr.Session{{Name: "stopped"}}
	if cmds := m.loadAllAgentsCmds(); len(cmds) != 0 {
		t.Fatalf("agent queries = %d, want none", len(cmds))
	}
}

func TestRemoteRunningSessionDoesNotQueueAgentInspection(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.sessions = []herdr.Session{{Target: "workbox", Name: "remote", Running: true}}
	if cmds := m.loadAllAgentsCmds(); len(cmds) != 0 {
		t.Fatalf("remote agent queries = %d, want none", len(cmds))
	}
}

func TestAgentRowsPreserveNestedAttachGuardAndBlockSessionActions(t *testing.T) {
	m := NewModel(Options{InsideHerdr: true}).(model)
	s := herdr.Session{Name: "work", Running: true}
	m.sessions = []herdr.Session{s}
	m.agents[s.ID()] = []herdr.Agent{{Name: "review", Target: "w1:p2", State: "working"}}
	m.configureTable()
	m.table.SetCursor(1)
	updated, cmd := m.attachAgent(s, m.agents[s.ID()][0])
	got := updated.(model)
	if cmd != nil || got.dialog == nil || got.dialog.Title != "Cannot attach from inside Herdr" {
		t.Fatal("agent attach bypassed nested guard")
	}
	got.dialog = nil
	updated, _ = got.confirmStop()
	got = updated.(model)
	if got.confirm != nil || got.dialog == nil {
		t.Fatal("stop was allowed from agent child row")
	}
}

func TestFavoritePersistsOnlyOnSuccess(t *testing.T) {
	store := &preferences.Store{Path: filepath.Join(t.TempDir(), "preferences.json")}
	m := NewModel(Options{PreferencesStore: store}).(model)
	m.preferencesPending = false
	m.setSessions([]herdr.Session{{Name: "api"}})
	next, cmd := m.toggleFavorite()
	m = next.(model)
	if !m.preferencesPending || m.preferences.Favorite("api") {
		t.Fatal("optimistic save or missing busy flag")
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	if !m.preferences.Favorite("api") || m.preferencesPending {
		t.Fatal("save not applied")
	}
	loaded, err := store.Load()
	if err != nil || !loaded.Favorite("api") {
		t.Fatal("not persisted")
	}
	next, _ = m.Update(preferencesLoadedMsg{ID: m.preferencesRequestID - 1, Data: preferences.Empty()})
	if !next.(model).preferences.Favorite("api") {
		t.Fatal("stale result applied")
	}
}

func TestDetailsLongContentFitsAndScrolls(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.width = 60
	m.height = 20
	m.setSessions([]herdr.Session{{Name: "api", SessionDir: strings.Repeat("/long-path", 100) + "\x1b[31m"}})
	m = m.openDetails()
	if strings.Contains(m.details.viewport.GetContent(), "\x1b[31m") {
		t.Fatal("unsafe content")
	}
	before := m.details.viewport.YOffset()
	m = navKey(m, "down")
	if m.details.viewport.YOffset() <= before {
		t.Fatal("did not scroll")
	}
}

func TestNavigationPopupsFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {80, 24}, {60, 20}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := NewModel(Options{DefaultDir: t.TempDir()}).(model)
			m.width = size[0]
			m.height = size[1]
			m.setSessions([]herdr.Session{{Name: "api", SessionDir: strings.Repeat("/long", 100)}})
			m = m.openDetails()
			check := func(view string) {
				if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
					t.Fatalf("popup exceeds %dx%d: %dx%d\n%s", m.width, m.height, lipgloss.Width(view), lipgloss.Height(view), view)
				}
			}
			check(m.overlayView())
		})
	}
}

func TestPreferenceFailurePreservesFavoriteAndOrdinaryActions(t *testing.T) {
	store := &preferences.Store{Path: filepath.Join(t.TempDir(), "preferences.json")}
	m := NewModel(Options{PreferencesStore: store}).(model)
	next, _ := m.Update(preferencesLoadedMsg{Initial: true, Err: errors.New("invalid JSON")})
	m = next.(model)
	if m.preferencesPending || m.preferencesError == nil || m.dialog == nil {
		t.Fatal("startup failure not reported")
	}
	m = navKey(m, "enter")
	m.setSessions([]herdr.Session{{Name: "api", Running: true}})
	m = navKey(m, "f")
	if m.filter != filterRunning {
		t.Fatal("ordinary navigation disabled")
	}
	next, cmd := m.toggleFavorite()
	m = next.(model)
	if cmd != nil || m.dialog == nil {
		t.Fatal("invalid store remained writable")
	}
	m.dialog = nil
	m.preferencesError = nil
	m.preferences.Favorites = []string{"api"}
	next, cmd = m.toggleFavorite()
	m = next.(model)
	if !m.preferences.Favorite("api") {
		t.Fatal("favorite removed before save")
	}
	next, _ = m.Update(preferencesLoadedMsg{ID: m.preferencesRequestID, Err: errors.New("write failed")})
	m = next.(model)
	if !m.preferences.Favorite("api") || m.preferencesPending {
		t.Fatal("failed save changed favorite")
	}
	_ = cmd
}

func TestFavoriteReordersWithoutChangingTarget(t *testing.T) {
	store := &preferences.Store{Path: filepath.Join(t.TempDir(), "preferences.json")}
	m := NewModel(Options{PreferencesStore: store}).(model)
	m.preferencesPending = false
	m.setSessions([]herdr.Session{{Name: "api", Running: true}, {Name: "web", Running: true}})
	m.table.SetCursor(1)
	next, cmd := m.toggleFavorite()
	m = next.(model)
	next, _ = m.Update(cmd())
	m = next.(model)
	selected, _ := m.selectedSession()
	if selected.Name != "web" || m.table.Cursor() != 0 {
		t.Fatal("pin changed target")
	}
	next, cmd = m.toggleFavorite()
	m = next.(model)
	next, _ = m.Update(cmd())
	m = next.(model)
	selected, _ = m.selectedSession()
	if selected.Name != "web" || m.table.Cursor() != 1 {
		t.Fatal("unpin changed target")
	}
	m = navKey(m, "s")
	m = navKey(m, "o")
	if m.confirm == nil || m.confirm.Session.Name != "web" || m.runningFirst {
		t.Fatal("confirmation lost input ownership")
	}
}

func TestRunningFirstWithinFavoritesAndStoppedFilter(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.preferences.Favorites = []string{"z-stopped"}
	m.setSessions([]herdr.Session{{Name: "a-stopped"}, {Name: "b-running", Running: true}, {Name: "z-stopped"}})
	m = navKey(m, "o")
	assertSessionNames(t, m.visibleSessions(), []string{"z-stopped", "b-running", "a-stopped"})
	m = navKey(m, "f")
	m = navKey(m, "f")
	assertSessionNames(t, m.visibleSessions(), []string{"z-stopped", "a-stopped"})
}

func TestDetailsKeepRefreshSpinnerScheduled(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{{Name: "work", Running: true}})
	m = m.openDetails()
	_, cmd := m.Update(m.spinner.Tick())
	if cmd == nil {
		t.Fatal("details discarded the next tick of the active refresh spinner")
	}
}

func TestExpandedHelpFitsSupportedTerminals(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {80, 24}, {120, 36}} {
		for _, busy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/busy=%t", size[0], size[1], busy), func(t *testing.T) {
				m := NewModel(Options{AllowStoppedAttach: true}).(model)
				m.loading = false
				sessions := make([]herdr.Session, 40)
				for i := range sessions {
					sessions[i].Name = fmt.Sprintf("session-%02d", i)
				}
				m.setSessions(sessions)
				updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				m = updated.(model)
				m.table.SetCursor(20)
				selected, _ := m.selectedSessionSnapshot()
				originalHeight := m.table.Height()
				if busy {
					m.busy = "test"
				}
				m = navKey(m, "?")
				for _, group := range m.sessionHelpKeys().FullHelp() {
					for _, binding := range group {
						if !strings.Contains(m.helpView(), binding.Help().Desc) {
							t.Errorf("missing help: %s", binding.Help().Desc)
						}
					}
				}
				if height := lipgloss.Height(m.render()); height > size[1] {
					t.Errorf("render height %d exceeds terminal height %d", height, size[1])
				}
				if name, _ := m.selectedSessionSnapshot(); name != selected {
					t.Fatal("opening help changed selection")
				}
				m = navKey(m, "?")
				if m.table.Height() != originalHeight {
					t.Fatal("closing help did not restore table height")
				}
			})
		}
	}
}

func TestExpandedHelpWithSearchFitsSmallTerminal(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.loading = false
	sessions := make([]herdr.Session, 20)
	for i := range sessions {
		sessions[i].Name = fmt.Sprintf("session-%02d", i)
	}
	m.setSessions(sessions)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = navKey(updated.(model), "?")
	m = navKey(m, "/")
	if height := lipgloss.Height(m.render()); height > 20 {
		t.Fatalf("search and help render %d lines in a 20-line terminal", height)
	}
}

func TestAgentRefreshPreservesSelectedSessionAndDiscardsStoppedResult(t *testing.T) {
	m := NewModel(Options{}).(model)
	first := herdr.Session{Name: "a", Running: true}
	second := herdr.Session{Name: "b", Running: true}
	m.setSessions([]herdr.Session{first, second})
	m.table.SetCursor(1)
	next, _ := m.Update(agentsLoadedMsg{ID: first.ID(), Agents: []herdr.Agent{{Name: "agent", Target: "w1:p1"}}})
	m = next.(model)
	selected, ok := m.selectedSession()
	if !ok || selected.Name != "b" || m.table.Cursor() != 2 {
		t.Fatalf("selection shifted after agent refresh: %#v cursor=%d", selected, m.table.Cursor())
	}
	first.Running = false
	m.setSessions([]herdr.Session{first, second})
	next, _ = m.Update(agentsLoadedMsg{ID: first.ID(), Agents: []herdr.Agent{{Name: "late", Target: "w1:p1"}}})
	m = next.(model)
	if len(m.agents[first.ID()]) != 0 {
		t.Fatal("stopped session accepted late agent data")
	}
}

func TestAgentRowsKeepCombinedHostLabelsOnParents(t *testing.T) {
	m := multiModel()
	local := herdr.Session{Name: "api", Running: true}
	m.agents[local.ID()] = []herdr.Agent{{Name: "review", Target: "w1:p2", State: "blocked", CWD: "/projects/herdrctx"}}
	m.configureTable()
	rows := m.pickerRows(m.visibleSessions())
	rendered := m.table.Rows()
	for i, row := range rows {
		if row.Agent != nil {
			if !strings.Contains(rendered[i][1], "needs in") || !strings.Contains(rendered[i][2], "herdrctx") {
				t.Fatalf("agent overwritten by host: %#v", rendered[i])
			}
		} else if !strings.Contains(rendered[i][2], hostName(row.Session.Target)) {
			t.Fatalf("wrong host: %#v", rendered[i])
		}
	}
}

func TestAgentRowsShowUsefulContext(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.width = 140
	s := herdr.Session{Name: "work", Running: true, SessionDir: "/storage/internal", SocketPath: "/storage/herdr.sock"}
	m.setSessions([]herdr.Session{s})
	agents, err := herdr.ParseAgents([]byte(`{"result":{"agents":[{"name":null,"agent":"codex","display_agent":"Codex","agent_status":"blocked","pane_id":"w1:p2","workspace_id":"w1","cwd":"/projects/old","foreground_cwd":"/projects/herdrctx","title":"Approve picker changes","terminal_title_stripped":"Fallback title"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	m.agents[s.ID()] = agents
	m.configureTable()
	rows := m.table.Rows()
	if ansi.Strip(rows[1][0]) != "  └ Codex" || ansi.Strip(rows[1][1]) != "  └ needs input" || ansi.Strip(rows[1][2]) != "herdrctx" || ansi.Strip(rows[1][3]) != "Approve picker changes" {
		t.Fatalf("agent context = %#v", rows[1])
	}
	if ansi.Strip(rows[0][2]) != "" || ansi.Strip(rows[0][3]) != "" {
		t.Fatalf("parent exposes storage internals: %#v", rows[0])
	}
	if m.table.Columns()[2].Title != "Project" || m.table.Columns()[3].Title != "Title" {
		t.Fatalf("headers = %#v", m.table.Columns())
	}
}

func TestAgentRowsUseReportedFallbacksWithoutPaneIDs(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.width = 140
	s := herdr.Session{Name: "work", Running: true}
	m.setSessions([]herdr.Session{s})
	agents, err := herdr.ParseAgents([]byte(`{"result":{"agents":[{"agent":"codex","agent_status":"working","pane_id":"w1:p2","cwd":"/projects/herdrctx","terminal_title_stripped":"Review session picker"},{"pane_id":"w1:p3","agent_status":"unknown"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	m.agents[s.ID()] = agents
	m.configureTable()
	rows := m.table.Rows()
	if ansi.Strip(rows[1][0]) != "  └ codex" || ansi.Strip(rows[1][2]) != "herdrctx" || ansi.Strip(rows[1][3]) != "Review session picker" {
		t.Fatalf("fallback context = %#v", rows[1])
	}
	if ansi.Strip(rows[2][0]) != "  └ Agent" || ansi.Strip(rows[2][2]) != "" || ansi.Strip(rows[2][3]) != "" {
		t.Fatalf("missing context = %#v", rows[2])
	}
	m.table.SetCursor(2)
	_, agent, ok := m.selectedAgent()
	if !ok || agent.Target != "w1:p3" {
		t.Fatalf("raw target lost: %#v", agent)
	}
}

func TestPickerColorsKeepAttentionAndSelectionReadable(t *testing.T) {
	m := NewModel(Options{}).(model)
	m.loading = false
	s := herdr.Session{Name: "work", Running: true}
	m.setSessions([]herdr.Session{s})
	m.agents[s.ID()] = []herdr.Agent{{Name: "question", Target: "w1:p1", State: "blocked"}, {Name: "ready", Target: "w1:p2", State: "idle"}}
	m.configureTable()
	rows := m.table.Rows()
	if !strings.Contains(rows[1][1], "245;169;127") || !strings.Contains(rows[1][1], "\x1b[1;") {
		t.Fatalf("needs input should be orange and bold: %q", rows[1][1])
	}
	if !strings.Contains(rows[2][1], "108;112;134") {
		t.Fatalf("idle should be muted: %q", rows[2][1])
	}
	m = navKey(m, "down")
	rows = m.table.Rows()
	if strings.Contains(strings.Join(rows[1], ""), "\x1b") {
		t.Fatalf("selected row has conflicting inner colors: %#v", rows[1])
	}
	if !strings.Contains(rows[0][0], "138;173;244") {
		t.Fatalf("session should be blue: %q", rows[0][0])
	}
	if ansi.Strip(rows[1][1]) != "  └ needs input" {
		t.Fatal("color changed the selected status text")
	}
}
