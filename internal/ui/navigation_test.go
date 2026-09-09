package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
