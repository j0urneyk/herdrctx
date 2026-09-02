package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/j0urneyk/herdrctx/internal/herdr"
)

func TestDefaultRefreshInterval(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	if m.refreshInterval != 3*time.Second {
		t.Fatalf("refreshInterval = %s, want 3s", m.refreshInterval)
	}
}

func TestRefreshIndicatorWaitsBeforeRendering(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.loading = false
	m.lastRefresh = time.Date(2026, 5, 31, 12, 34, 56, 0, time.UTC)
	cmd := m.startRefresh()
	if cmd == nil {
		t.Fatal("startRefresh() cmd = nil, want delayed indicator command")
	}
	if strings.Contains(m.summaryLine(), "refreshing") {
		t.Fatalf("summaryLine() = %q, want refreshing hidden before delay", m.summaryLine())
	}

	updated, _ := m.Update(refreshIndicatorMsg(m.refreshIndicatorID))
	got := updated.(model)
	if !strings.Contains(got.summaryLine(), "refreshing") {
		t.Fatalf("summaryLine() = %q, want refreshing after delay", got.summaryLine())
	}
}

func TestRefreshIndicatorStaysHiddenWhenLoadFinishesBeforeDelay(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.loading = false
	_ = m.startRefresh()
	id := m.refreshIndicatorID

	updated, _ := m.Update(sessionsLoadedMsg{RefreshedAt: time.Now()})
	got := updated.(model)
	updated, _ = got.Update(refreshIndicatorMsg(id))
	got = updated.(model)
	if got.showRefreshing {
		t.Fatal("showRefreshing = true, want false after load finishes")
	}
	if strings.Contains(got.summaryLine(), "refreshing") {
		t.Fatalf("summaryLine() = %q, want refreshing hidden after load finishes", got.summaryLine())
	}
}

func TestRefreshIndicatorIgnoresStaleDelay(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.loading = false
	_ = m.startRefresh()
	staleID := m.refreshIndicatorID
	_ = m.startRefresh()

	updated, _ := m.Update(refreshIndicatorMsg(staleID))
	got := updated.(model)
	if got.showRefreshing {
		t.Fatal("showRefreshing = true, want stale delay ignored")
	}

	updated, _ = got.Update(refreshIndicatorMsg(got.refreshIndicatorID))
	got = updated.(model)
	if !got.showRefreshing {
		t.Fatal("showRefreshing = false, want current delay to show indicator")
	}
}

func TestStaleRefreshResponseIsIgnored(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.loading = false
	m.setSessions([]herdr.Session{{Name: "current"}})
	_ = m.startRefresh()
	staleID := m.refreshRequestID
	_ = m.startRefresh()
	currentID := m.refreshRequestID
	refreshedAt := time.Date(2026, 5, 31, 12, 34, 56, 0, time.UTC)

	updated, _ := m.Update(sessionsLoadedMsg{
		RequestID:   currentID,
		Sessions:    []herdr.Session{{Name: "new"}},
		RefreshedAt: refreshedAt,
	})
	got := updated.(model)
	updated, _ = got.Update(sessionsLoadedMsg{
		RequestID:   staleID,
		Sessions:    []herdr.Session{{Name: "stale"}},
		Err:         errors.New("old refresh failed"),
		RefreshedAt: refreshedAt.Add(-time.Minute),
	})
	got = updated.(model)

	if len(got.sessions) != 1 || got.sessions[0].Name != "new" {
		t.Fatalf("sessions = %#v, want current refresh result", got.sessions)
	}
	if got.statusKind != statusSuccess {
		t.Fatalf("statusKind = %v, want success from current refresh", got.statusKind)
	}
	if !got.lastRefresh.Equal(refreshedAt) {
		t.Fatalf("lastRefresh = %s, want %s", got.lastRefresh, refreshedAt)
	}
}

func TestSetSessionsPreservesSelectionByName(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{{Name: "default"}, {Name: "work"}})
	m.table.SetCursor(1)

	m.setSessions([]herdr.Session{{Name: "work"}, {Name: "side"}})

	selected, ok := m.selectedSession()
	if !ok {
		t.Fatal("selectedSession() ok = false")
	}
	if selected.Name != "work" {
		t.Fatalf("selected session = %q, want work", selected.Name)
	}
}

func TestSearchFiltersSessionsByName(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions(searchFixtureSessions())
	m.search.input.SetValue("api")
	m.configureTable()

	assertSessionNames(t, m.visibleSessions(), []string{"api"})
}

func TestSearchFiltersSessionsByDirectory(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions(searchFixtureSessions())
	m.search.scope = sessionSearchScopeDirectory
	m.search.updatePrompt()
	m.search.input.SetValue("clients")
	m.configureTable()

	assertSessionNames(t, m.visibleSessions(), []string{"web"})
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions(searchFixtureSessions())
	m.search.scope = sessionSearchScopeDirectory
	m.search.updatePrompt()
	m.search.input.SetValue("REPO")
	m.configureTable()

	assertSessionNames(t, m.visibleSessions(), []string{"api", "web"})
}

func TestSearchTabSwitchesScopeAndKeepsQuery(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{
		{Name: "repo-name", SessionDir: "/tmp/other"},
		{Name: "other", SessionDir: "/tmp/repo"},
	})
	m.search.input.SetValue("repo")
	m.configureTable()

	updated, _ := m.handleKey(keyPress("/"))
	got := updated.(model)
	updated, cmd := got.handleKey(keyPress("tab"))
	got = updated.(model)
	if cmd != nil {
		t.Fatal("search tab command = non-nil, want nil")
	}
	if got.search.scope != sessionSearchScopeDirectory {
		t.Fatalf("search scope = %s, want directory", got.search.scope)
	}
	if got.search.query() != "repo" {
		t.Fatalf("search query = %q, want repo", got.search.query())
	}
	assertSessionNames(t, got.visibleSessions(), []string{"other"})
}

func TestSearchEnterClosesInputAndKeepsFilter(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions(searchFixtureSessions())
	m.search.input.SetValue("api")
	m.configureTable()

	updated, _ := m.handleKey(keyPress("/"))
	got := updated.(model)
	updated, cmd := got.handleKey(keyPress("enter"))
	got = updated.(model)
	if cmd != nil {
		t.Fatal("search enter command = non-nil, want nil")
	}
	if got.search.active {
		t.Fatal("search active after enter = true, want false")
	}
	if got.search.query() != "api" {
		t.Fatalf("search query = %q, want api", got.search.query())
	}
	assertSessionNames(t, got.visibleSessions(), []string{"api"})
	if strings.Contains(got.summaryLine(), "search:") {
		t.Fatalf("summaryLine() = %q, want search state outside summary", got.summaryLine())
	}
	if !strings.Contains(got.render(), `Search name: "api"`) {
		t.Fatalf("render() missing retained search bar:\n%s", got.render())
	}
}

func TestSearchBarRendersAboveSessionList(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.width = 100
	m.height = 30
	m.loading = false
	m.setSessions(searchFixtureSessions())
	m.search.input.SetValue("api")
	m.search.active = true
	m.configureTable()

	view := m.render()
	searchIndex := strings.Index(view, "Search name:")
	tableIndex := strings.Index(view, "Directory")
	if searchIndex < 0 {
		t.Fatalf("render() missing search bar:\n%s", view)
	}
	if tableIndex < 0 {
		t.Fatalf("render() missing session table:\n%s", view)
	}
	if searchIndex > tableIndex {
		t.Fatalf("search bar should render above session table:\n%s", view)
	}
	for _, border := range []string{"╭", "╰"} {
		if !strings.Contains(view, border) {
			t.Fatalf("render() missing active search border %q:\n%s", border, view)
		}
	}
}

func TestSearchEscClearsFilter(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions(searchFixtureSessions())
	m.search.input.SetValue("api")
	m.configureTable()

	updated, _ := m.handleKey(keyPress("/"))
	got := updated.(model)
	updated, cmd := got.handleKey(keyPress("esc"))
	got = updated.(model)
	if cmd != nil {
		t.Fatal("search esc command = non-nil, want nil")
	}
	if got.search.active {
		t.Fatal("search active after esc = true, want false")
	}
	if got.search.query() != "" {
		t.Fatalf("search query = %q, want empty", got.search.query())
	}
	assertSessionNames(t, got.visibleSessions(), []string{"api", "web", "db"})
}

func TestFilteredSelectionDrivesActions(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{
		{Name: "api", Running: false},
		{Name: "web", Running: false},
	})
	m.search.input.SetValue("web")
	m.configureTable()

	selected, ok := m.selectedSession()
	if !ok {
		t.Fatal("selectedSession() ok = false")
	}
	if selected.Name != "web" {
		t.Fatalf("selected session = %q, want web", selected.Name)
	}

	updated, cmd := m.confirmDelete()
	got := updated.(model)
	if cmd != nil {
		t.Fatal("confirmDelete() command = non-nil, want nil")
	}
	if got.confirm == nil {
		t.Fatal("confirm = nil, want confirmation")
	}
	if got.confirm.Session.Name != "web" {
		t.Fatalf("confirmation session = %q, want web", got.confirm.Session.Name)
	}
}

func TestSearchFilterPersistsAcrossRefresh(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{{Name: "api"}, {Name: "web"}})
	m.search.input.SetValue("api")
	m.configureTable()

	m.setSessions([]herdr.Session{{Name: "web"}, {Name: "api"}, {Name: "db"}})

	if m.search.query() != "api" {
		t.Fatalf("search query = %q, want api", m.search.query())
	}
	assertSessionNames(t, m.visibleSessions(), []string{"api"})
	selected, ok := m.selectedSession()
	if !ok {
		t.Fatal("selectedSession() ok = false")
	}
	if selected.Name != "api" {
		t.Fatalf("selected session after refresh = %q, want api", selected.Name)
	}
}

func TestSearchNoMatchRenderDiffersFromEmptySessionList(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.width = 100
	m.height = 30
	m.loading = false
	m.setSessions(searchFixtureSessions())
	m.search.input.SetValue("missing")
	m.configureTable()

	view := m.render()
	if !strings.Contains(view, `No sessions match name search "missing".`) {
		t.Fatalf("render() missing no-match search message:\n%s", view)
	}
	if strings.Contains(view, "No Herdr sessions found.") {
		t.Fatalf("render() used empty-list message for filtered no-match:\n%s", view)
	}
}

func TestSearchHelpBindings(t *testing.T) {
	t.Parallel()

	keys := defaultKeyMap()
	if !bindingHelpExists(keys.ShortHelp(), "/", "search") {
		t.Fatalf("ShortHelp() = %#v, want search binding", keys.ShortHelp())
	}
	if !fullHelpBindingExists(keys.FullHelp(), "tab", "search scope") {
		t.Fatalf("FullHelp() = %#v, want search scope binding", keys.FullHelp())
	}
}

func TestSessionRowsSanitizeTerminalControlSequences(t *testing.T) {
	t.Parallel()

	rows := sessionRows([]herdr.Session{{
		Name:       "work\x1b[2J",
		Running:    true,
		SessionDir: "/tmp/\x1b]52;c;secret\adir",
		SocketPath: "/tmp/\x00sock",
	}}, []table.Column{
		{Width: 30},
		{Width: 10},
		{Width: 30},
		{Width: 30},
	})
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}

	row := strings.Join(rows[0], "|")
	for _, forbidden := range []string{"\x1b", "[2J", "secret", "\x00"} {
		if strings.Contains(row, forbidden) {
			t.Fatalf("row contains %q after sanitization: %q", forbidden, row)
		}
	}
	if !strings.Contains(row, "work") || !strings.Contains(row, "/tmp/dir") || !strings.Contains(row, "/tmp/sock") {
		t.Fatalf("row lost safe display text: %q", row)
	}
}

func TestSessionRowsDimStoppedSessions(t *testing.T) {
	t.Parallel()

	rows := sessionRows([]herdr.Session{
		{Name: "running", Running: true, SessionDir: "/tmp/running"},
		{Name: "stopped", Running: false, SessionDir: "/tmp/stopped"},
	}, []table.Column{
		{Width: 30},
		{Width: 10},
		{Width: 30},
		{Width: 30},
	})
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	runningRow := strings.Join(rows[0], "|")
	stoppedRow := strings.Join(rows[1], "|")
	if strings.Contains(runningRow, "\x1b") {
		t.Fatalf("running row contains ANSI styling, want unstyled row: %q", runningRow)
	}
	if !strings.Contains(stoppedRow, "\x1b") {
		t.Fatalf("stopped row = %q, want ANSI dim styling", stoppedRow)
	}
	if !strings.Contains(stoppedRow, "stopped") {
		t.Fatalf("stopped row lost safe display text: %q", stoppedRow)
	}
}

func TestSelectedStoppedSessionRowUsesSelectedStyleOnly(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{
		{Name: "running", Running: true},
		{Name: "stopped", Running: false},
	})
	m.table.MoveDown(1)
	m.refreshTableRowsForCurrentCursor()

	rows := m.table.Rows()
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	selectedStoppedRow := strings.Join(rows[1], "|")
	if strings.Contains(selectedStoppedRow, "\x1b") {
		t.Fatalf("selected stopped row contains inner ANSI styling, want table selected style to own selection: %q", selectedStoppedRow)
	}
}

func TestDeleteDefaultSessionIsBlocked(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{{Name: "default", Default: true}})

	updated, cmd := m.confirmDelete()
	got := updated.(model)
	if cmd != nil {
		t.Fatal("confirmDelete() returned command, want nil")
	}
	if got.confirm != nil {
		t.Fatalf("confirm = %#v, want nil", got.confirm)
	}
	if got.statusKind != statusWarning {
		t.Fatalf("statusKind = %v, want warning", got.statusKind)
	}
	if got.dialog == nil {
		t.Fatal("dialog = nil, want warning dialog")
	}
}

func TestDeleteRunningSessionRequiresStopFirst(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{{Name: "work", Running: true}})

	updated, cmd := m.confirmDelete()
	got := updated.(model)
	if cmd != nil {
		t.Fatal("confirmDelete() returned command, want nil")
	}
	if got.confirm != nil {
		t.Fatalf("confirm = %#v, want nil", got.confirm)
	}
	if got.statusKind != statusWarning {
		t.Fatalf("statusKind = %v, want warning", got.statusKind)
	}
	if got.dialog == nil {
		t.Fatal("dialog = nil, want warning dialog")
	}
}

func TestStopJSONFlagSessionNameIsBlocked(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{{Name: "--json", Running: true}})

	updated, cmd := m.confirmStop()
	got := updated.(model)
	if cmd != nil {
		t.Fatal("confirmStop() command = non-nil, want nil")
	}
	if got.confirm != nil {
		t.Fatalf("confirm = %#v, want nil", got.confirm)
	}
	if got.dialog == nil {
		t.Fatal("dialog = nil, want warning dialog")
	}
	if got.dialog.Title != "Session cannot be stopped" {
		t.Fatalf("dialog title = %q, want stop warning", got.dialog.Title)
	}
}

func TestDeleteJSONFlagSessionNameIsBlocked(t *testing.T) {
	t.Parallel()

	for _, running := range []bool{false, true} {
		running := running
		t.Run(fmt.Sprintf("running-%t", running), func(t *testing.T) {
			t.Parallel()

			m := NewModel(Options{}).(model)
			m.setSessions([]herdr.Session{{Name: "--json", Running: running}})

			updated, cmd := m.confirmDelete()
			got := updated.(model)
			if cmd != nil {
				t.Fatal("confirmDelete() command = non-nil, want nil")
			}
			if got.confirm != nil {
				t.Fatalf("confirm = %#v, want nil", got.confirm)
			}
			if got.dialog == nil {
				t.Fatal("dialog = nil, want warning dialog")
			}
			if got.dialog.Title != "Session cannot be deleted" {
				t.Fatalf("dialog title = %q, want delete warning", got.dialog.Title)
			}
		})
	}
}

func TestDeleteConfirmationRechecksCurrentSessionState(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{{Name: "work", Running: false}})
	m.confirm = &confirmation{Action: confirmDelete, Session: herdr.Session{Name: "work", Running: false}}
	m.setSessions([]herdr.Session{{Name: "work", Running: true}})

	updated, cmd := m.handleConfirmationKey(keyPress("enter"))
	got := updated.(model)
	if cmd != nil {
		t.Fatal("handleConfirmationKey() command = non-nil, want nil")
	}
	if got.busy != "" {
		t.Fatalf("busy = %q, want empty", got.busy)
	}
	if got.dialog == nil {
		t.Fatal("dialog = nil, want stale-state warning")
	}
	if got.dialog.Title != "Stop session before deleting" {
		t.Fatalf("dialog title = %q, want running-session warning", got.dialog.Title)
	}
}

func TestDeleteActionRefreshesCurrentSessionState(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("shell fake is Unix-only")
	}

	dir := t.TempDir()
	marker := filepath.Join(dir, "deleted")
	client := herdr.NewClient(fakeHerdrCommand(t, fmt.Sprintf(`
if [ "$1" = "session" ] && [ "$2" = "list" ] && [ "$3" = "--json" ]; then
  printf '{"sessions":[{"default":false,"name":"work","running":true,"session_dir":"/tmp/work","socket_path":"/tmp/work/herdr.sock"}]}'
  exit 0
fi
if [ "$1" = "session" ] && [ "$2" = "delete" ]; then
  printf deleted > %s
  exit 0
fi
exit 2
`, shellQuote(marker))))

	m := NewModel(Options{Client: client}).(model)
	msg := m.runActionCmd(confirmation{Action: confirmDelete, Session: herdr.Session{Name: "work", Running: false}})().(actionFinishedMsg)
	if msg.Err == nil {
		t.Fatal("delete action error = nil, want stale running-state error")
	}
	if !strings.Contains(msg.Err.Error(), "before deleting") {
		t.Fatalf("delete action error = %v, want stopped-session error", msg.Err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("delete marker exists, want DeleteSession not invoked")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat delete marker: %v", err)
	}
}

func TestConfirmationEscStillCancels(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.confirm = &confirmation{Action: confirmStop, Session: herdr.Session{Name: "work", Running: true}}

	updated, cmd := m.handleKey(keyPress("esc"))
	got := updated.(model)
	if cmd != nil {
		t.Fatal("Esc confirmation command = non-nil, want nil")
	}
	if got.confirm != nil {
		t.Fatalf("confirm = %#v, want nil", got.confirm)
	}
	if got.status != "Action cancelled." {
		t.Fatalf("status = %q, want cancellation status", got.status)
	}
}

func TestCtrlCQuitsFromModalAndBusyLayers(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "projects"))
	newSessionModel := NewModel(Options{DefaultDir: base}).(model)
	updated, _ := newSessionModel.openNewSession(newSessionWithDir)
	newSessionModel = updated.(model)
	newSessionForm := *newSessionModel.newSession
	newSessionForm.nextField()
	newSessionForm.dir.SetValue("pro")
	newSessionForm.refreshDirectoryCompletion()
	newSessionModel.newSession = &newSessionForm

	tests := []struct {
		name string
		m    model
	}{
		{
			name: "dialog",
			m: func() model {
				m := NewModel(Options{}).(model)
				m.showDialog(dialogWarning, "Warning", "Something happened.")
				return m
			}(),
		},
		{
			name: "new-session",
			m:    newSessionModel,
		},
		{
			name: "busy",
			m: func() model {
				m := NewModel(Options{}).(model)
				m.busy = `stop "work"`
				return m
			}(),
		},
		{
			name: "confirmation",
			m: func() model {
				m := NewModel(Options{}).(model)
				m.confirm = &confirmation{Action: confirmStop, Session: herdr.Session{Name: "work", Running: true}}
				return m
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, cmd := tt.m.handleKey(keyPress("ctrl+c"))
			assertQuitCommand(t, cmd)
		})
	}
}

func TestQuitCancelsCommandContext(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	_, cmd := m.handleKey(keyPress("q"))
	assertQuitCommand(t, cmd)

	select {
	case <-m.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("command context was not canceled")
	}
}

func TestAttachInsideHerdrIsBlocked(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{InsideHerdr: true, CurrentSocketPath: "/tmp/herdr.sock"}).(model)
	m.setSessions([]herdr.Session{{Name: "default", Running: true, SocketPath: "/tmp/herdr.sock"}})

	updated, cmd := m.attachSelected()
	got := updated.(model)
	if cmd != nil {
		t.Fatal("attachSelected() returned command inside Herdr, want nil")
	}
	if got.statusKind != statusWarning {
		t.Fatalf("statusKind = %v, want warning", got.statusKind)
	}
	if got.dialog == nil {
		t.Fatal("dialog = nil, want nested warning dialog")
	}
	if !strings.Contains(got.dialog.Body, "Already inside") {
		t.Fatalf("dialog body = %q, want already-inside warning", got.dialog.Body)
	}
}

func TestAllowNestedBypassesAttachBlock(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{InsideHerdr: true, AllowNested: true}).(model)
	m.setSessions([]herdr.Session{{Name: "work", Running: true}})

	_, cmd := m.attachSelected()
	if cmd == nil {
		t.Fatal("attachSelected() command = nil, want ExecProcess command")
	}
}

func TestAttachHelpSessionNameIsBlocked(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"help", "--help"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := NewModel(Options{}).(model)
			m.setSessions([]herdr.Session{{Name: name, Running: true}})

			updated, cmd := m.attachSelected()
			got := updated.(model)
			if cmd != nil {
				t.Fatal("attachSelected() command = non-nil, want nil")
			}
			if got.dialog == nil {
				t.Fatal("dialog = nil, want warning dialog")
			}
			if got.dialog.Title != "Session cannot be attached" {
				t.Fatalf("dialog title = %q, want attach warning", got.dialog.Title)
			}
		})
	}
}

func TestNewSessionRejectsInvalidNames(t *testing.T) {
	t.Parallel()

	for _, mode := range []newSessionMode{newSessionQuick, newSessionWithDir} {
		for _, name := range []string{"-", "--", "-work", "--json", "--session", "--help", "-h", "_work", ".work", "help"} {
			t.Run(fmt.Sprintf("%d/%s", mode, name), func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				missingDir := filepath.Join(dir, "new-directory")
				m := NewModel(Options{DefaultDir: dir}).(model)
				updated, _ := m.openNewSession(mode)
				got := updated.(model)
				got.newSession.name.SetValue(name)
				if mode == newSessionWithDir {
					got.newSession.dir.SetValue(missingDir)
					got.newSession.active = newSessionDirField
				}

				updated, cmd := got.submitNewSession()
				got = updated.(model)
				if cmd == nil {
					t.Fatal("submitNewSession() cmd = nil, want focus command")
				}
				if got.newSession == nil {
					t.Fatal("newSession = nil, want form to remain open")
				}
				wantError := "must start with an ASCII letter or number"
				if name == "help" {
					wantError = "reserved by Herdr"
				}
				if !strings.Contains(got.newSession.err, wantError) {
					t.Fatalf("form error = %q, want %q", got.newSession.err, wantError)
				}
				if got.newSession.active != newSessionNameField || got.newSession.name.Value() != name {
					t.Fatal("want name input focused with the rejected name preserved")
				}
				if got.dialog != nil || got.busy != "" {
					t.Fatal("want inline validation without a dialog or active command")
				}
				if _, err := os.Stat(missingDir); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("stat start directory = %v, want directory not created", err)
				}
			})
		}
	}
}

func TestOpenNewSessionInsideHerdrIsBlocked(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{InsideHerdr: true, DefaultDir: t.TempDir()}).(model)
	updated, cmd := m.openNewSession(newSessionQuick)
	got := updated.(model)
	if cmd != nil {
		t.Fatal("openNewSession() returned command inside Herdr, want nil")
	}
	if got.newSession != nil {
		t.Fatal("newSession form opened inside Herdr, want blocked")
	}
	if got.statusKind != statusWarning {
		t.Fatalf("statusKind = %v, want warning", got.statusKind)
	}
	if got.dialog == nil {
		t.Fatal("dialog = nil, want nested warning dialog")
	}
	if got.dialog.Title != "Cannot create from inside Herdr" {
		t.Fatalf("dialog title = %q, want create warning", got.dialog.Title)
	}
	if !strings.Contains(got.dialog.Body, "Creating a session attaches immediately") {
		t.Fatalf("dialog body = %q, want create-and-attach warning", got.dialog.Body)
	}
	if strings.Contains(got.dialog.Body, "Already inside") {
		t.Fatalf("dialog body = %q, want create warning without current-session attach text", got.dialog.Body)
	}
}

func TestNewSessionShortcutInsideHerdrShowsCreateDialog(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{InsideHerdr: true, CurrentSocketPath: "/tmp/herdr.sock", DefaultDir: t.TempDir()}).(model)
	m.setSessions([]herdr.Session{{Name: "default", Running: true, SocketPath: "/tmp/herdr.sock"}})

	updated, cmd := m.handleKey(keyPress("n"))
	got := updated.(model)
	if cmd != nil {
		t.Fatal("handleKey(n) returned command inside Herdr, want nil")
	}
	if got.newSession != nil {
		t.Fatal("newSession form opened inside Herdr, want blocked")
	}
	if got.dialog == nil {
		t.Fatal("dialog = nil, want nested warning dialog")
	}
	if got.dialog.Title != "Cannot create from inside Herdr" {
		t.Fatalf("dialog title = %q, want create warning", got.dialog.Title)
	}
	if !strings.Contains(got.dialog.Body, "Creating a session attaches immediately") {
		t.Fatalf("dialog body = %q, want create-and-attach warning", got.dialog.Body)
	}
	if strings.Contains(got.dialog.Body, "Already inside") {
		t.Fatalf("dialog body = %q, want create warning without selected-session attach text", got.dialog.Body)
	}
}

func TestOpenQuickNewSession(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{DefaultDir: t.TempDir()}).(model)
	updated, cmd := m.openNewSession(newSessionQuick)
	got := updated.(model)

	if cmd == nil {
		t.Fatal("openNewSession() cmd = nil, want focus command")
	}
	if got.newSession == nil {
		t.Fatal("newSession = nil")
	}
	if got.newSession.withDir() {
		t.Fatal("quick new session unexpectedly has directory field")
	}
}

func TestOpenNewSessionWithDir(t *testing.T) {
	t.Parallel()

	defaultDir := t.TempDir()
	m := NewModel(Options{DefaultDir: defaultDir}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)

	if got.newSession == nil {
		t.Fatal("newSession = nil")
	}
	if !got.newSession.withDir() {
		t.Fatal("new session with dir did not enable directory field")
	}
	if got.newSession.dir.Value() != defaultDir {
		t.Fatalf("dir = %q, want %q", got.newSession.dir.Value(), defaultDir)
	}
}

func TestNewSessionWithDirRejectsEmptyDirectory(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{DefaultDir: t.TempDir()}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	got.newSession.name.SetValue("work")
	got.newSession.dir.SetValue("")

	updated, cmd := got.submitNewSession()
	got = updated.(model)
	if cmd == nil {
		t.Fatal("submitNewSession() cmd = nil, want focus command")
	}
	if got.newSession == nil {
		t.Fatal("newSession = nil, want form to remain open")
	}
	if got.newSession.err == "" {
		t.Fatal("form error is empty, want directory error")
	}
}

func TestNewSessionWithDirCreatesMissingDirectory(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	dir := filepath.Join(base, "new", "child")
	m := NewModel(Options{DefaultDir: base}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	got.newSession.name.SetValue("work")
	got.newSession.dir.SetValue(filepath.Join("new", "child"))

	updated, cmd := got.submitNewSession()
	got = updated.(model)
	if cmd == nil {
		t.Fatal("submitNewSession() cmd = nil, want exec command")
	}
	if got.newSession != nil {
		t.Fatalf("newSession = %#v, want nil after submit", got.newSession)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat created directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("created path is not a directory")
	}
}

func TestNewSessionDirectoryCompletionAcceptsSelection(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "projects"))
	mustMkdir(t, filepath.Join(base, "prototypes"))

	m := NewModel(Options{DefaultDir: base}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	form := *got.newSession
	form.nextField()
	_ = form.focusActive()
	form.dir.SetValue("pro")
	form.refreshDirectoryCompletion()

	form, cmd := form.update(keyPress("down"), defaultKeyMap())
	if cmd != nil {
		t.Fatal("update() command = non-nil, want nil")
	}
	form, cmd = form.update(keyPress("tab"), defaultKeyMap())
	if cmd != nil {
		t.Fatal("update() command = non-nil, want nil")
	}
	if got := form.dir.Value(); got != "prototypes/" {
		t.Fatalf("dir value = %q, want prototypes/", got)
	}
	if form.completion.open() {
		t.Fatal("completion open after accept, want closed")
	}

	form, cmd = form.update(keyPress("up"), defaultKeyMap())
	if cmd == nil {
		t.Fatal("up after accept command = nil, want focus command")
	}
	if form.active != newSessionNameField {
		t.Fatalf("active field after accept+up = %v, want name field", form.active)
	}
}

func TestNewSessionDirectoryCompletionEscClosesDropdownBeforeDialog(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "projects"))
	mustMkdir(t, filepath.Join(base, "prototypes"))

	m := NewModel(Options{DefaultDir: base}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	form := *got.newSession
	form.nextField()
	form.dir.SetValue("pro")
	form.refreshDirectoryCompletion()
	if !form.completion.open() {
		t.Fatal("completion closed before Esc, want open")
	}
	got.newSession = &form

	updated, cmd := got.handleKey(keyPress("esc"))
	got = updated.(model)
	if cmd != nil {
		t.Fatal("first Esc command = non-nil, want nil")
	}
	if got.newSession == nil {
		t.Fatal("newSession = nil after first Esc, want form to remain open")
	}
	if got.newSession.completion.open() {
		t.Fatal("completion open after first Esc, want closed")
	}

	updated, cmd = got.handleKey(keyPress("esc"))
	got = updated.(model)
	if cmd != nil {
		t.Fatal("second Esc command = non-nil, want nil")
	}
	if got.newSession != nil {
		t.Fatalf("newSession = %#v after second Esc, want nil", got.newSession)
	}
	if got.status != "New session cancelled." {
		t.Fatalf("status = %q, want cancellation status", got.status)
	}
}

func TestNewSessionCompletionLayerNavigatesAndAcceptsSelection(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "projects"))
	mustMkdir(t, filepath.Join(base, "prototypes"))

	m := NewModel(Options{DefaultDir: base}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	form := *got.newSession
	form.nextField()
	form.dir.SetValue("pro")
	form.refreshDirectoryCompletion()
	got.newSession = &form

	updated, cmd := got.handleKey(keyPress("down"))
	got = updated.(model)
	if cmd != nil {
		t.Fatal("down command = non-nil, want nil")
	}
	item, ok := got.newSession.completion.selectedItem()
	if !ok {
		t.Fatal("selectedItem() ok = false")
	}
	if item.Value != "prototypes/" {
		t.Fatalf("selected item = %q, want prototypes/", item.Value)
	}

	updated, cmd = got.handleKey(keyPress("tab"))
	got = updated.(model)
	if cmd != nil {
		t.Fatal("tab command = non-nil, want nil")
	}
	if got.newSession.dir.Value() != "prototypes/" {
		t.Fatalf("dir value = %q, want prototypes/", got.newSession.dir.Value())
	}
	if got.newSession.completion.open() {
		t.Fatal("completion open after tab, want closed")
	}
}

func TestNewSessionCompletionLayerPassesTextInputToForm(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "projects"))

	m := NewModel(Options{DefaultDir: base}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	form := *got.newSession
	form.nextField()
	_ = form.focusActive()
	form.dir.SetValue("pro")
	form.dir.CursorEnd()
	form.refreshDirectoryCompletion()
	if !form.completion.open() {
		t.Fatal("completion closed before text input, want open")
	}
	got.newSession = &form

	updated, _ = got.handleKey(keyPress("x"))
	got = updated.(model)
	if got.newSession == nil {
		t.Fatal("newSession = nil after text input, want form to remain open")
	}
	if got.newSession.dir.Value() != "prox" {
		t.Fatalf("dir value = %q, want prox", got.newSession.dir.Value())
	}
}

func TestNewSessionArrowKeysMoveFieldsWhenCompletionClosed(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{DefaultDir: t.TempDir()}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	form := *got.newSession

	form, cmd := form.update(keyPress("down"), defaultKeyMap())
	if cmd == nil {
		t.Fatal("down from name command = nil, want focus command")
	}
	if form.active != newSessionDirField {
		t.Fatalf("active field = %v, want directory field", form.active)
	}
	if form.completion.open() {
		t.Fatal("completion opened on field focus, want closed")
	}

	form, cmd = form.update(keyPress("up"), defaultKeyMap())
	if cmd == nil {
		t.Fatal("up from directory command = nil, want focus command")
	}
	if form.active != newSessionNameField {
		t.Fatalf("active field = %v, want name field", form.active)
	}
}

func TestNewSessionDirectoryCompletionTabDoesNotMoveWithoutCandidates(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{DefaultDir: t.TempDir()}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	form := *got.newSession
	form.nextField()
	form.dir.SetValue("missing")
	form.refreshDirectoryCompletion()

	form, cmd := form.update(keyPress("tab"), defaultKeyMap())
	if cmd != nil {
		t.Fatal("tab without candidates command = non-nil, want nil")
	}
	if form.active != newSessionDirField {
		t.Fatalf("active field = %v, want directory field", form.active)
	}
}

func TestNewSessionDirectoryCompletionRendersDropdown(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "projects"))

	m := NewModel(Options{DefaultDir: base}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	form := *got.newSession
	form.nextField()
	form.dir.SetValue("pro")
	form.refreshDirectoryCompletion()

	view := form.render(100)
	if !strings.Contains(view, "› projects/") {
		t.Fatalf("dropdown render missing selected candidate:\n%s", view)
	}
	if !strings.Contains(view, "Tab completes") {
		t.Fatalf("dropdown render missing completion help:\n%s", view)
	}
}

func TestNewSessionDirectoryCompletionReopensAfterAcceptedValueChanges(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "projects", "xray"))

	m := NewModel(Options{DefaultDir: base}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	form := *got.newSession
	form, cmd := form.update(keyPress("down"), defaultKeyMap())
	if cmd == nil {
		t.Fatal("down command = nil, want focus command")
	}
	form.dir.SetValue("pro")
	form.refreshDirectoryCompletion()

	form, cmd = form.update(keyPress("tab"), defaultKeyMap())
	if cmd != nil {
		t.Fatal("tab command = non-nil, want nil")
	}
	if form.completion.open() {
		t.Fatal("completion open after accept, want closed")
	}

	form, _ = form.update(keyPress("x"), defaultKeyMap())
	if !form.completion.open() {
		t.Fatal("completion closed after editing accepted value, want open")
	}
	item, ok := form.completion.selectedItem()
	if !ok {
		t.Fatal("selectedItem() ok = false")
	}
	if item.Value != "projects/xray/" {
		t.Fatalf("selected item = %q, want projects/xray/", item.Value)
	}
}

func TestNewSessionDirectoryCompletionAcceptsBeyondVisibleWindow(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	for i := range 10 {
		mustMkdir(t, filepath.Join(base, fmt.Sprintf("dir%02d", i)))
	}

	m := NewModel(Options{DefaultDir: base}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	form := *got.newSession
	form.nextField()
	form.dir.SetValue("dir")
	form.refreshDirectoryCompletion()

	for range 8 {
		var cmd tea.Cmd
		form, cmd = form.update(keyPress("down"), defaultKeyMap())
		if cmd != nil {
			t.Fatal("update() command = non-nil, want nil")
		}
	}
	form, cmd := form.update(keyPress("tab"), defaultKeyMap())
	if cmd != nil {
		t.Fatal("update() command = non-nil, want nil")
	}
	if got := form.dir.Value(); got != "dir08/" {
		t.Fatalf("dir value = %q, want dir08/", got)
	}
}

func TestNewSessionDirectoryCompletionUsesConfiguredVisibleCount(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	for i := range 5 {
		mustMkdir(t, filepath.Join(base, fmt.Sprintf("dir%02d", i)))
	}

	m := NewModel(Options{DefaultDir: base, CompleteVisibleCount: 3}).(model)
	updated, _ := m.openNewSession(newSessionWithDir)
	got := updated.(model)
	form := *got.newSession
	form.nextField()
	form.dir.SetValue("dir")
	form.refreshDirectoryCompletion()

	if got := form.completion.effectiveVisibleLimit(); got != 3 {
		t.Fatalf("visible limit = %d, want 3", got)
	}
	view := form.render(100)
	if !strings.Contains(view, "1-3 of 5") {
		t.Fatalf("render missing configured visible count:\n%s", view)
	}
}

func TestDialogCloseKey(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.showDialog(dialogWarning, "Warning", "Something happened.")

	updated, cmd := m.handleKey(keyPress("enter"))
	got := updated.(model)
	if cmd != nil {
		t.Fatal("handleKey() command = non-nil, want nil")
	}
	if got.dialog != nil {
		t.Fatalf("dialog = %#v, want nil", got.dialog)
	}
}

func TestDialogBlocksBackgroundActions(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.setSessions([]herdr.Session{{Name: "work", Running: false}})
	m.showDialog(dialogWarning, "Warning", "Something happened.")

	updated, cmd := m.handleKey(keyPress("d"))
	got := updated.(model)
	if cmd != nil {
		t.Fatal("handleKey() command = non-nil, want nil")
	}
	if got.dialog == nil {
		t.Fatal("dialog = nil, want it to remain open")
	}
	if got.confirm != nil {
		t.Fatalf("confirm = %#v, want nil", got.confirm)
	}
}

func TestBusyStateBlocksActions(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.loading = false
	m.busy = `stop "work"`
	m.setSessions([]herdr.Session{{Name: "work", Running: false}})

	updated, cmd := m.handleKey(keyPress("d"))
	got := updated.(model)
	if cmd != nil {
		t.Fatal("delete while busy command = non-nil, want nil")
	}
	if got.confirm != nil {
		t.Fatalf("confirm = %#v, want nil while busy", got.confirm)
	}

	updated, cmd = got.handleKey(keyPress("r"))
	got = updated.(model)
	if cmd != nil {
		t.Fatal("refresh while busy command = non-nil, want nil")
	}
	if got.loading {
		t.Fatal("loading = true after refresh while busy, want false")
	}
}

func TestRefreshKeyIsIgnoredWhileLoading(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.loading = true
	requestID := m.refreshRequestID

	updated, cmd := m.handleKey(keyPress("r"))
	got := updated.(model)
	if cmd != nil {
		t.Fatal("refresh while loading command = non-nil, want nil")
	}
	if got.refreshRequestID != requestID {
		t.Fatalf("refreshRequestID = %d, want %d", got.refreshRequestID, requestID)
	}
	if got.status != "Refresh already in progress." {
		t.Fatalf("status = %q, want in-progress status", got.status)
	}
}

func TestDialogRendersAsOverlay(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	m.width = 100
	m.height = 30
	m.setSessions([]herdr.Session{{Name: "default", Running: true}})
	m.showDialog(dialogWarning, "Warning", "Something happened.")

	view := m.render()
	if !strings.Contains(view, "Directory") {
		t.Fatalf("dialog overlay should preserve the table behind it:\n%s", view)
	}
	if !strings.Contains(view, "Warning") {
		t.Fatalf("dialog render missing title:\n%s", view)
	}
	if !strings.Contains(view, "Press Enter") {
		t.Fatalf("dialog render missing close help:\n%s", view)
	}
	assertOverlayPlacement(t, view, "Warning")
}

func TestConfirmationRendersAsOverlay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		session herdr.Session
		open    func(model) (tea.Model, tea.Cmd)
		needle  string
	}{
		{
			name:    "stop",
			session: herdr.Session{Name: "work", Running: true},
			open:    func(m model) (tea.Model, tea.Cmd) { return m.confirmStop() },
			needle:  `Stop session "work"?`,
		},
		{
			name:    "delete",
			session: herdr.Session{Name: "work", Running: false},
			open:    func(m model) (tea.Model, tea.Cmd) { return m.confirmDelete() },
			needle:  `Delete session "work"?`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := NewModel(Options{}).(model)
			m.width = 100
			m.height = 30
			m.setSessions([]herdr.Session{tt.session})

			updated, cmd := tt.open(m)
			got := updated.(model)
			if cmd != nil {
				t.Fatal("confirmation returned command, want nil")
			}

			view := got.render()
			if !strings.Contains(view, "Directory") {
				t.Fatalf("confirmation overlay should preserve the table behind it:\n%s", view)
			}
			if !strings.Contains(view, "Press y or Enter") {
				t.Fatalf("confirmation render missing action help:\n%s", view)
			}
			assertOverlayPlacement(t, view, tt.needle)
		})
	}
}

func TestNewSessionRendersAsOverlay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mode   newSessionMode
		needle string
	}{
		{name: "quick", mode: newSessionQuick, needle: "New session"},
		{name: "with-dir", mode: newSessionWithDir, needle: "New session in directory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := NewModel(Options{DefaultDir: t.TempDir()}).(model)
			m.width = 100
			m.height = 30
			m.setSessions([]herdr.Session{{Name: "work", Running: true}})

			updated, cmd := m.openNewSession(tt.mode)
			got := updated.(model)
			if cmd == nil {
				t.Fatal("openNewSession() cmd = nil, want focus command")
			}

			view := got.render()
			if !strings.Contains(view, "Directory") {
				t.Fatalf("new-session overlay should preserve the table behind it:\n%s", view)
			}
			assertOverlayPlacement(t, view, tt.needle)
		})
	}
}

func TestNewSessionRenderSanitizesFormError(t *testing.T) {
	t.Parallel()

	form, _ := newSessionFormModel(newSessionWithDir, t.TempDir(), 100, CompletionHiddenAuto, 8)
	form.err = "bad\x1b]52;c;secret\aerror"

	view := form.render(100)
	if strings.Contains(view, "\x1b]52") || strings.Contains(view, "secret") {
		t.Fatalf("form render contains control sequence: %q", view)
	}
	if !strings.Contains(view, "baderror") {
		t.Fatalf("form render = %q, want sanitized error text", view)
	}
}

func TestNewSessionRenderSanitizesInputValues(t *testing.T) {
	t.Parallel()

	form, _ := newSessionFormModel(newSessionWithDir, "safe", 100, CompletionHiddenAuto, 8)
	form.name.SetValue("bad\x1b]52;c;secret\aname")
	form.dir.SetValue("dir\x1b[2J")

	view := form.render(100)
	for _, forbidden := range []string{"\x1b]52", "\x1b[2J", "\a"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("form render contains %q: %q", forbidden, view)
		}
	}
	if !strings.Contains(view, "bad") || !strings.Contains(view, "dir") {
		t.Fatalf("form render = %q, want sanitized input text", view)
	}
}

func TestRefreshFailureUsesStatusOnly(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	updated, _ := m.Update(sessionsLoadedMsg{Err: errors.New("herdr missing"), RefreshedAt: time.Now()})
	got := updated.(model)
	if got.dialog != nil {
		t.Fatalf("dialog = %#v, want nil for refresh failure", got.dialog)
	}
	if got.statusKind != statusError {
		t.Fatalf("statusKind = %v, want error", got.statusKind)
	}
}

func TestRefreshFailureSanitizesStatus(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	updated, _ := m.Update(sessionsLoadedMsg{Err: errors.New("bad\x1b[2Jstatus"), RefreshedAt: time.Now()})
	got := updated.(model)
	if strings.Contains(got.statusView(), "\x1b[2J") {
		t.Fatalf("statusView() contains control sequence: %q", got.statusView())
	}
	if !strings.Contains(got.statusView(), "badstatus") {
		t.Fatalf("statusView() = %q, want sanitized text", got.statusView())
	}
}

func TestActionFailureShowsErrorDialog(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	updated, _ := m.Update(actionFinishedMsg{Action: confirmStop, Name: "work", Err: errors.New("stop failed")})
	got := updated.(model)
	if got.dialog == nil {
		t.Fatal("dialog = nil, want error dialog")
	}
	if got.dialog.Kind != dialogError {
		t.Fatalf("dialog kind = %v, want error", got.dialog.Kind)
	}
}

func TestActionFailureDialogSanitizesBody(t *testing.T) {
	t.Parallel()

	m := NewModel(Options{}).(model)
	updated, _ := m.Update(actionFinishedMsg{Action: confirmStop, Name: "work", Err: errors.New("bad\x1b]52;c;secret\adialog")})
	got := updated.(model)
	if got.dialog == nil {
		t.Fatal("dialog = nil, want error dialog")
	}
	view := got.dialog.render(100)
	if strings.Contains(view, "\x1b]52") || strings.Contains(view, "secret") {
		t.Fatalf("dialog render contains control sequence: %q", view)
	}
	if !strings.Contains(view, "baddialog") {
		t.Fatalf("dialog render = %q, want sanitized text", view)
	}
}

func TestQuickNewSessionSubmitUsesDefaultDir(t *testing.T) {
	t.Parallel()

	defaultDir := t.TempDir()
	m := NewModel(Options{DefaultDir: defaultDir}).(model)
	updated, _ := m.openNewSession(newSessionQuick)
	got := updated.(model)
	got.newSession.name.SetValue("work")

	updated, cmd := got.submitNewSession()
	got = updated.(model)
	if cmd == nil {
		t.Fatal("submitNewSession() cmd = nil, want exec command")
	}
	if got.newSession != nil {
		t.Fatalf("newSession = %#v, want nil after submit", got.newSession)
	}
	if got.busy != `new session "work"` {
		t.Fatalf("busy = %q", got.busy)
	}
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;:]*[A-Za-z]`)

func assertOverlayPlacement(t *testing.T, view string, needle string) {
	t.Helper()

	line, column, ok := visiblePosition(view, needle)
	if !ok {
		t.Fatalf("render missing %q:\n%s", needle, view)
	}
	if line > 16 {
		t.Fatalf("%q rendered too low for a centered overlay: line=%d column=%d\n%s", needle, line, column, view)
	}
	if column < 6 {
		t.Fatalf("%q rendered too far left for a centered overlay: line=%d column=%d\n%s", needle, line, column, view)
	}
}

func visiblePosition(view string, needle string) (int, int, bool) {
	for i, line := range strings.Split(view, "\n") {
		visible := ansiEscapePattern.ReplaceAllString(line, "")
		if column := strings.Index(visible, needle); column >= 0 {
			return i + 1, column + 1, true
		}
	}

	return 0, 0, false
}

func assertQuitCommand(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("command = nil, want quit command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("command message = %T, want tea.QuitMsg", msg)
	}
}

func fakeHerdrCommand(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "herdr")
	content := "#!/bin/sh\n" + body + "\n"
	// Forks wait until the executable's writable descriptor is closed.
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	// #nosec G306 -- The fake Herdr command must be executable for the test.
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake herdr: %v", err)
	}

	return path
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func searchFixtureSessions() []herdr.Session {
	return []herdr.Session{
		{Name: "api", SessionDir: "/home/me/repo/api"},
		{Name: "web", SessionDir: "/home/me/repo/clients/web"},
		{Name: "db", SessionDir: "/var/lib/db"},
	}
}

func assertSessionNames(t *testing.T, sessions []herdr.Session, want []string) {
	t.Helper()

	got := sessionNames(sessions)
	if !slices.Equal(got, want) {
		t.Fatalf("session names = %v, want %v", got, want)
	}
}

func sessionNames(sessions []herdr.Session) []string {
	names := make([]string, 0, len(sessions))
	for _, session := range sessions {
		names = append(names, session.Name)
	}

	return names
}

func bindingHelpExists(bindings []key.Binding, helpKey string, desc string) bool {
	for _, binding := range bindings {
		help := binding.Help()
		if help.Key == helpKey && help.Desc == desc {
			return true
		}
	}

	return false
}

func fullHelpBindingExists(groups [][]key.Binding, helpKey string, desc string) bool {
	for _, group := range groups {
		if bindingHelpExists(group, helpKey, desc) {
			return true
		}
	}

	return false
}

func keyPress(value string) tea.KeyPressMsg {
	switch value {
	case "ctrl+c":
		return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
	case "down":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
	case "shift+tab":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift})
	case "tab":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab})
	case "up":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	default:
		runes := []rune(value)
		if len(runes) == 0 {
			return tea.KeyPressMsg(tea.Key{})
		}
		return tea.KeyPressMsg(tea.Key{Text: value, Code: runes[0]})
	}
}
