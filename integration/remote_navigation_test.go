//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type remoteNavigationState struct {
	Sessions []session `json:"sessions"`
	Fail     bool      `json:"fail"`
	Hold     bool      `json:"hold"`
}

func fixtureRemoteHerdr(args []string) error {
	if slices.Equal(args, []string{"--version"}) {
		fmt.Println("herdr 0.8.2")
		return nil
	}
	if len(args) != 4 || args[0] != "--remote" || args[1] != "fake-host" || args[2] != "--session" {
		return fmt.Errorf("unexpected foreground arguments: %q", args)
	}
	if err := recordRemoteAction(args); err != nil {
		return err
	}
	fmt.Println("REMOTE_NAVIGATION_HANDOFF")
	return nil
}

func recordRemoteAction(args []string) error {
	raw, err := json.Marshal(invocation{Args: args})
	if err != nil {
		return err
	}
	return appendFile(filepath.Join(os.Getenv("HERDRCTX_TEST_ROOT"), "remote-actions.jsonl"), append(raw, '\n'))
}

func fixtureRemoteSSH(args []string) error {
	separator := slices.Index(args, "--")
	if separator < 0 || len(args) != separator+3 || args[separator+1] != "fake-host" {
		return fmt.Errorf("unexpected SSH arguments: %q", args)
	}
	command := args[separator+2]
	root := os.Getenv("HERDRCTX_TEST_ROOT")
	if command != "herdr --version && herdr session list --json" {
		return recordRemoteAction([]string{command})
	}
	// #nosec G304 G703 -- The parent supplies this isolated fixture root.
	lock, err := os.OpenFile(filepath.Join(root, "query.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	fd, err := descriptorInt(lock.Fd())
	if err != nil {
		return err
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = appendFile(filepath.Join(root, "overlap.log"), []byte("overlapping SSH queries\n"))
		return err
	}
	// #nosec G304 G703 -- The parent supplies the isolated fixture root and state.
	raw, err := os.ReadFile(filepath.Join(root, "remote-state.json"))
	if err != nil {
		return err
	}
	var state remoteNavigationState
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if err := appendFile(filepath.Join(root, "queries.log"), []byte("start\n")); err != nil {
		return err
	}
	if state.Hold {
		if err := waitFor(context.Background(), 10*time.Second, func() bool {
			// #nosec G703 -- This is the parent's fixed synchronization file in the fixture root.
			_, err := os.Stat(filepath.Join(root, "release-query"))
			return err == nil
		}); err != nil {
			return err
		}
	}
	if state.Fail {
		return fmt.Errorf("remote fixture offline")
	}
	fmt.Println("herdr 0.8.2")
	_, err = os.Stdout.Write(raw)
	return err
}

type remoteNavigation struct {
	s     *scenario
	state remoteNavigationState
	herdr string
}

func newRemoteNavigation(t *testing.T) *remoteNavigation {
	t.Helper()
	s := newScenario(t, "remote-navigation")
	ssh := s.fixture("fake-ssh")
	must(t, os.Rename(ssh, filepath.Join(s.root, "bin", "ssh")))
	s.env["PATH"] = filepath.Join(s.root, "bin") + string(os.PathListSeparator) + s.env["PATH"]
	n := &remoteNavigation{s: s, herdr: s.fixture("remote-herdr"), state: remoteNavigationState{Sessions: []session{{Name: "api", Running: true}}}}
	n.save()
	return n
}

func (n *remoteNavigation) save() {
	atomicJSON(n.s.t, filepath.Join(n.s.root, "remote-state.json"), n.state)
}

func (n *remoteNavigation) start(label string, extra ...string) *terminal {
	args := []string{"--remote", "fake-host", "--herdr-bin", n.herdr, "--interval", "500ms"}
	p := n.s.terminal(label, repoPath(*binaryFlag), append(args, extra...)...)
	p.expect("Loaded 1 session(s).", 0)
	return p
}

func (n *remoteNavigation) actions() []invocation {
	raw, err := os.ReadFile(filepath.Join(n.s.root, "remote-actions.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	must(n.s.t, err)
	var actions []invocation
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		actions = append(actions, decodeJSON[invocation](n.s.t, line))
	}
	return actions
}

func (n *remoteNavigation) queries() int {
	raw, _ := os.ReadFile(filepath.Join(n.s.root, "queries.log"))
	return strings.Count(string(raw), "start\n")
}

func TestRemoteNavigation(t *testing.T) {
	t.Run("modal-and-confirmation", func(t *testing.T) {
		n := newRemoteNavigation(t)
		p := n.start("modal")
		p.sendExpect("i", "Host: fake-host")
		p.send("ansdN")
		p.sendExpect(esc, "enter/a")
		if len(n.actions()) != 0 {
			t.Fatal("dialog leaked an action")
		}
		p.sendExpect("N", "Remote directory creation unavailable")
		p.sendExpect(enter, "enter/a")
		p.sendExpect("s", `Stop session "api" on fake-host?`)
		if len(n.actions()) != 0 {
			t.Fatal("stop ran without confirmation")
		}
		p.sendExpect("n", "enter/a")
		p.sendExpect("s", `Stop session "api" on fake-host?`)
		p.send(enter)
		n.s.wait("one confirmed stop", func() bool { return len(n.actions()) == 1 })
		p.quit()
		if n.actions()[0].Args[0] != "herdr session stop 'api' --json" {
			t.Fatal(n.actions())
		}
	})
	t.Run("stopped-and-nested", func(t *testing.T) {
		n := newRemoteNavigation(t)
		n.state.Sessions[0].Running = false
		n.save()
		p := n.start("stopped")
		p.sendExpect(enter, "Stopped session attach disabled")
		p.sendExpect(enter, "enter/a")
		p.quit()
		if len(n.actions()) != 0 {
			t.Fatal("stopped attach invoked Herdr")
		}
		p = n.start("allowed", "--allow-stopped-attach")
		p.sendExpect(enter, "REMOTE_NAVIGATION_HANDOFF")
		p.expect("enter/a", p.after("REMOTE_NAVIGATION_HANDOFF"))
		p.quit()
		if !slices.Equal(n.actions()[0].Args, []string{"--remote", "fake-host", "--session", "api"}) {
			t.Fatal(n.actions())
		}
		for _, env := range []string{"HERDR_ENV", "HERDR_SOCKET_PATH"} {
			n.s.env[env] = "1"
			p = n.start(env)
			p.sendExpect(enter, "Cannot attach from inside Herdr")
			p.sendExpect(enter, "enter/a")
			for _, key := range []string{"n", "N"} {
				p.sendExpect(key, "Cannot create from inside Herdr")
				p.sendExpect(enter, "enter/a")
			}
			p.quit()
			delete(n.s.env, env)
		}
		if len(n.actions()) != 1 {
			t.Fatal("nested guard invoked Herdr")
		}
	})
	t.Run("refresh-serialization-and-cancel", func(t *testing.T) {
		n := newRemoteNavigation(t)
		p := n.start("serialization")
		n.state.Fail = true
		n.save()
		p.expect("Refresh failed", 0)
		count := n.queries()
		n.state.Fail = false
		n.state.Hold = true
		n.save()
		n.s.wait("blocked background query", func() bool { return n.queries() > count })
		count = n.queries()
		p.sendExpect(enter, "Checking remote session")
		p.sendExpect(esc, "Remote check cancelled")
		p.sendExpect(enter, "Checking remote session")
		n.state.Hold = false
		n.state.Sessions[0].Running = false
		n.save()
		writeFile(t, filepath.Join(n.s.root, "release-query"), []byte("release"), 0o600)
		p.expect("Stopped session attach disabled", 0)
		if n.queries() <= count {
			t.Fatal("old background result was reused")
		}
		if len(n.actions()) != 0 {
			t.Fatal("cancelled or stale action invoked Herdr")
		}
		p.sendExpect(enter, "enter/a")
		p.quit()
		if _, err := os.Stat(filepath.Join(n.s.root, "overlap.log")); !os.IsNotExist(err) {
			t.Fatal("SSH queries overlapped")
		}
	})
	t.Run("confirmation-refresh-failure", func(t *testing.T) {
		n := newRemoteNavigation(t)
		p := n.start("stale-confirmation")
		p.sendExpect("s", `Stop session "api" on fake-host?`)
		n.state.Fail = true
		n.save()
		// Observe a failed query without relying on the hidden background status line.
		count := n.queries()
		n.s.wait("failure while confirmation open", func() bool { return n.queries() > count+1 })
		p.sendExpect(enter, "Remote check failed")
		if len(n.actions()) != 0 {
			t.Fatal("stale confirmation ran stop")
		}
		p.sendExpect(enter, "enter/a")
		p.quit()
	})
	t.Run("cancel-active-check", func(t *testing.T) {
		n := newRemoteNavigation(t)
		p := n.start("active-cancel", "--interval", "30s")
		n.state.Fail = true
		n.save()
		p.sendExpect("r", "Refresh failed")
		n.state.Fail = false
		n.state.Hold = true
		n.save()
		count := n.queries()
		p.sendExpect(enter, "Checking remote session")
		n.s.wait("active held check", func() bool { return n.queries() > count })
		p.sendExpect(esc, "Remote check cancelled")
		n.state.Hold = false
		n.state.Sessions[0].Running = false
		n.save()
		p.sendExpect(enter, "Stopped session attach disabled")
		p.sendExpect(enter, "enter/a")
		p.quit()
		if len(n.actions()) != 0 {
			t.Fatal("cancelled check attached")
		}
		if _, err := os.Stat(filepath.Join(n.s.root, "overlap.log")); !os.IsNotExist(err) {
			t.Fatal("cancelled SSH process overlapped its successor")
		}
	})
}
