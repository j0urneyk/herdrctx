//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/j0urneyk/herdrctx/internal/preferences"
)

func fixtureFleetHerdr(args []string) error {
	if slices.Equal(args, []string{"--version"}) {
		fmt.Println("herdr 0.9.0")
		return nil
	}
	if slices.Equal(args, []string{"machine", "list", "--json"}) {
		// #nosec G703 -- The test parent supplies the isolated fixture root and fixed filename.
		raw, err := os.ReadFile(filepath.Join(os.Getenv("HERDRCTX_TEST_ROOT"), "work", "machines.json"))
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(raw)
		return err
	}
	if len(args) == 4 && args[0] == "--remote" && args[2] == "--session" {
		if err := recordRemoteAction(args); err != nil {
			return err
		}
		fmt.Println("FLEET_HANDOFF_" + args[1])
		return nil
	}
	return fixtureHerdr(args)
}

func fixtureFleetSSH(args []string) error {
	i := slices.Index(args, "--")
	if i < 0 || len(args) != i+3 {
		return fmt.Errorf("unexpected args %q", args)
	}
	target := args[i+1]
	if target != "one" && target != "two" {
		return fmt.Errorf("unexpected target %q", target)
	}
	path := filepath.Join(os.Getenv("HERDRCTX_TEST_ROOT"), "work", target+".json")
	// #nosec G304 G703 -- The path uses the isolated root and one of two allowlisted target names.
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var state remoteNavigationState
	if err = json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if state.Fail {
		return fmt.Errorf("offline %s", target)
	}
	command := args[i+2]
	if command == "herdr --version && herdr session list --json" {
		fmt.Println("herdr 0.9.0")
		_, err = os.Stdout.Write(raw)
		return err
	}
	if err = recordRemoteAction([]string{target, command}); err != nil {
		return err
	}
	switch command {
	case "herdr session stop 'api' --json":
		state.Sessions[0].Running = false
	case "herdr session delete 'api' --json":
		state.Sessions = nil
	default:
		return fmt.Errorf("unexpected command %s", command)
	}
	raw, err = json.Marshal(state)
	if err != nil {
		return err
	}
	// #nosec G703 -- Write only the allowlisted target state in the isolated fixture root.
	return os.WriteFile(path, raw, 0o600)
}

func TestHostsNavigation(t *testing.T) {
	s := newScenario(t, "hosts-navigation")
	herdr := s.fixture("fleet-herdr")
	ssh := s.fixture("fleet-ssh")
	must(t, os.Rename(ssh, filepath.Join(s.root, "bin", "ssh")))
	s.env["PATH"] = filepath.Join(s.root, "bin") + string(os.PathListSeparator) + s.env["PATH"]
	state := remoteNavigationState{Sessions: []session{{Name: "api", Running: true}}}
	for _, target := range []string{"one", "two"} {
		atomicJSON(t, filepath.Join(s.root, "work", target+".json"), state)
	}
	atomicJSON(t, filepath.Join(s.root, "sessions.json"), sessionList{Sessions: state.Sessions})
	store := &preferences.HostStore{Path: filepath.Join(s.root, "hosts.json")}
	for _, target := range []string{"one", "two"} {
		_, err := store.Apply(preferences.HostChange{Host: preferences.Host{Label: strings.ToUpper(target), Destination: target}})
		must(t, err)
	}
	atomicJSON(t, filepath.Join(s.root, "work", "machines.json"), []map[string]any{{"id": "profile-one", "label": "Imported", "target": "one", "session": "api", "enabled": true, "selected": true}})
	p := s.terminal("fleet", repoPath(*binaryFlag), "--all-hosts", "--hosts-file", store.Path, "--herdr-bin", herdr, "--interval", "500ms")
	p.expect("two: 1 sessions", 0)
	p.sendExpect("H", "Hosts")
	p.send("\x1b[B")
	p.send(enter)
	p.sendExpect("i", "Host: one")
	p.sendExpect(enter, "enter/a")
	p.sendExpect(enter, "FLEET_HANDOFF_one")
	p.expect("enter/a", p.after("FLEET_HANDOFF_one"))
	p.sendExpect("H", "Hosts")
	p.send("A")
	p.sendExpect("n", "Choose a host for the new session")
	p.sendExpect(esc, "enter/a")
	p.sendExpect("H", "Hosts")
	p.send("\x1b[B\x1b[B")
	p.send(enter)
	p.sendExpect("s", `Stop session "api" on two?`)
	p.sendExpect("n", "enter/a")
	p.sendExpect("H", "Hosts")
	p.sendExpect("i", "Import a machine as a saved host")
	p.sendExpect(enter, "Replace ONE")
	p.send("y")
	s.wait("imported metadata saved", func() bool {
		d, err := store.Load()
		return err == nil && d.Items[0].ProfileID == "profile-one" && d.Items[0].Session == "api"
	})
	p.sendExpect(esc, "enter/a")
	p.sendExpect("H", "Hosts")
	p.sendExpect("n", "SSH destination")
	p.send("Three\tthree")
	p.send(enter)
	s.wait("host added", func() bool { d, err := store.Load(); return err == nil && len(d.Items) == 3 })
	p.sendExpect(esc, "enter/a")
	p.quit()
	raw := readFile(t, filepath.Join(s.root, "remote-actions.jsonl"))
	if strings.Contains(string(raw), "session stop") {
		t.Fatal("cancelled stop executed")
	}
	s.check("host selection, same-name handoff, explicit creation destination, stop cancellation, import conflict and add")
}
