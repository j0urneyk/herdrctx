//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

const (
	up    = "\x1b[A"
	down  = "\x1b[B"
	esc   = "\x1b"
	enter = "\r"
)

func TestNavigation(t *testing.T) {
	s := newScenario(t, "navigation")
	herdr := s.fixture("herdr")
	statePath := filepath.Join(s.root, "sessions.json")
	makeSession := func(name string, running bool) session {
		dir := filepath.Join(s.root, "session-state", name)
		return session{Name: name, Running: running, Default: name == "default", SessionDir: dir, SocketPath: filepath.Join(dir, "herdr.sock")}
	}
	state := sessionList{Sessions: []session{makeSession("alpha-running", true), makeSession("beta-stopped", false), makeSession("default", false), makeSession("gamma-running", true)}}
	atomicJSON(t, statePath, state)
	args := []string{"--herdr-bin", herdr, "--interval", "500ms"}
	start := func(label string, extra ...string) *terminal {
		return s.terminal(label, repoPath(*binaryFlag), append(slices.Clone(args), extra...)...)
	}
	details := func(p *terminal, name string) { p.sendExpect("i", "Name: "+name) }
	readPrefs := func() []string {
		data := decodeJSON[struct{ Favorites []string }](t, string(readFile(t, s.preferencesPath())))
		return data.Favorites
	}
	waitPrefs := func(names []string) {
		s.wait("saved favorites", func() bool {
			raw, err := os.ReadFile(s.preferencesPath())
			if os.IsNotExist(err) {
				return false
			}
			must(t, err)
			var data struct{ Favorites []string }
			must(t, json.Unmarshal(raw, &data))
			return slices.Equal(data.Favorites, names)
		})
	}
	actions := func() []invocation {
		raw, err := os.ReadFile(filepath.Join(s.root, "actions.jsonl"))
		if os.IsNotExist(err) {
			return nil
		}
		must(t, err)
		var result []invocation
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			result = append(result, decodeJSON[invocation](t, line))
		}
		return result
	}
	wantAttach := invocation{Args: []string{"session", "attach", "beta-stopped"}, CWD: filepath.Join(s.root, "work")}

	p := start("navigation")
	p.expect("Loaded 4 session(s).", 0)
	s.check("status filters, sort order, and selected action target")
	details(p, "alpha-running")
	p.send(enter)
	p.sendExpect("o", "Running first")
	p.send(down)
	details(p, "gamma-running")
	p.send(enter)
	p.sendExpect("o", "Name")
	p.send(strings.Repeat(up, 5) + down)
	details(p, "beta-stopped")
	p.send(enter)
	p.sendExpect("f", "Running")
	p.sendExpect("/alpha"+enter, "1/4")
	details(p, "alpha-running")
	p.send("nsdbpfo")
	p.sendExpect(esc, "enter/a")
	p.sendExpect("/"+esc, "gamma-running")
	p.sendExpect("f", "Stopped")
	p.sendExpect("f", "All")
	if len(actions()) != 0 {
		t.Fatalf("overlay invoked session action: %v", actions())
	}

	s.check("favorite persistence, lock failure, and retry")
	p.sendExpect("/beta"+enter, "1/4")
	p.sendExpect("p", "⭐\ufe0f")
	waitPrefs([]string{"beta-stopped"})
	lock, err := os.OpenFile(s.preferencesPath()+".lock", os.O_CREATE|os.O_WRONLY, 0o600)
	must(t, err)
	t.Cleanup(func() { _ = lock.Close() })
	fd, err := descriptorInt(lock.Fd())
	must(t, err)
	must(t, unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB))
	p.sendExpect("p", "Preferences unavailable")
	if !slices.Equal(readPrefs(), []string{"beta-stopped"}) {
		t.Fatal("failed save changed favorites")
	}
	must(t, lock.Close())
	p.send(enter)
	p.quit()
	p = start("restored")
	p.expect("Loaded 4 session(s).", 0)
	details(p, "beta-stopped")
	p.send(enter)
	p.send("p")
	waitPrefs(nil)
	p.quit()
	p = start("unfavorited")
	p.expect("Loaded 4 session(s).", 0)
	p.send(down)

	s.check("live details, disappearance, refresh failure, recovery, and resize")
	details(p, "beta-stopped")
	state.Sessions[1].Running = true
	offset := p.offset()
	atomicJSON(t, statePath, state)
	p.expect("running", offset)
	removed := sessionList{Sessions: []session{state.Sessions[0], state.Sessions[2], state.Sessions[3]}}
	offset = p.offset()
	atomicJSON(t, statePath, removed)
	p.expect("Session no longer listed", offset)
	removed.Fail = true
	offset = p.offset()
	atomicJSON(t, statePath, removed)
	p.expect("Refresh failed", offset)
	state.Sessions[1].SessionDir = filepath.Join(s.root, "RECOVERED-STATE")
	state.Sessions[1].SocketPath = s.root + strings.Repeat("/segment", 120) + "/END-SOCKET-PATH"
	offset = p.offset()
	atomicJSON(t, statePath, state)
	p.expect("RECOVERED-STATE", offset)
	p.sendExpect("\x1b[6~", "END-SOCKET-PATH")
	offset = p.offset()
	p.resize(60, 20)
	p.expect("Enter/Esc/q closes", offset)
	p.sendExpect(esc, "enter/a")
	p.resize(120, 36)

	s.check("unbound b key and ordinary foreground attach")
	p.sendExpect("/beta"+enter, "1/4")
	p.send("b")
	details(p, "beta-stopped")
	p.sendExpect(enter, "enter/a")
	p.sendExpect("a", "NAVIGATION_ATTACH_HANDOFF")
	p.expect("enter/a", p.after("NAVIGATION_ATTACH_HANDOFF"))
	if !reflect.DeepEqual(actions(), []invocation{wantAttach}) {
		t.Fatalf("unexpected foreground actions: %v", actions())
	}
	p.quit()

	s.check("corrupt startup preferences preserve data and session controls")
	const broken = "{invalid"
	writeFile(t, s.preferencesPath(), []byte(broken), 0o600)
	p = start("corrupt-preferences")
	p.expect("Preferences unavailable", 0)
	p.send(enter)
	p.sendExpect("n", "New session")
	p.sendExpect(esc, "enter/a")
	p.sendExpect("p", "Preferences unavailable")
	if string(readFile(t, s.preferencesPath())) != broken {
		t.Fatal("modified broken preferences")
	}
	p.send(enter)
	p.quit()
	if len(actions()) != 1 {
		t.Fatalf("unexpected actions: %v", actions())
	}

	s.check("stopped attach default, flag, environment, and explicit override")
	must(t, os.Remove(s.preferencesPath()))
	atomicJSON(t, statePath, sessionList{Sessions: []session{makeSession("beta-stopped", false)}})
	for _, tc := range []struct {
		name  string
		flags []string
		env   string
		allow bool
	}{
		{name: "blocked-default"},
		{name: "allowed-flag", flags: []string{"--allow-stopped-attach"}, allow: true},
		{name: "allowed-env", env: "1", allow: true},
		{name: "blocked-override", flags: []string{"--allow-stopped-attach=false"}, env: "1"},
	} {
		s.env["HERDRCTX_ALLOW_STOPPED_ATTACH"] = tc.env
		p = start(tc.name, tc.flags...)
		p.expect("Loaded ", 0)
		help, expected := "attach disabled", "Stopped session attach disabled"
		if tc.allow {
			help, expected = "start and attach", "NAVIGATION_ATTACH_HANDOFF"
		}
		p.expect(help, 0)
		count := len(actions())
		p.sendExpect(enter, expected)
		if tc.allow {
			p.expect("enter/a", p.after("NAVIGATION_ATTACH_HANDOFF"))
			if !reflect.DeepEqual(actions()[count:], []invocation{wantAttach}) {
				t.Fatalf("%s: unexpected actions: %v", tc.name, actions())
			}
		} else {
			p.sendExpect(enter, "attach disabled")
			if len(actions()) != count {
				t.Fatalf("%s invoked Herdr", tc.name)
			}
		}
		p.quit()
	}
	s.metadata["actions"] = actions()
	s.metadata["terminal_sizes"] = []string{"160x40", "120x36", "60x20"}
}
