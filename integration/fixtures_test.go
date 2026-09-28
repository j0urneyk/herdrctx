//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

type session struct {
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	Default    bool   `json:"default"`
	SessionDir string `json:"session_dir"`
	SocketPath string `json:"socket_path"`
}

type sessionList struct {
	AgentTargets []string  `json:"agent_targets,omitempty"`
	AgentTitle   string    `json:"agent_title,omitempty"`
	Sessions     []session `json:"sessions"`
	Fail         bool      `json:"fail_list,omitempty"`
}

type invocation struct {
	Args []string `json:"argv"`
	CWD  string   `json:"cwd"`
}

func (s *scenario) fixture(name string) string {
	executable, err := os.Executable()
	must(s.t, err)
	path := filepath.Join(s.root, "bin", name)
	writeFile(s.t, path, []byte("#!/bin/sh\nHERDRCTX_TEST_HELPER="+quote(name)+" exec "+quote(executable)+" \"$@\"\n"), 0o700)
	return path
}

func runHelper(mode string, args []string) int {
	var err error
	switch mode {
	case "fleet-herdr":
		err = fixtureFleetHerdr(args)
	case "fleet-ssh":
		err = fixtureFleetSSH(args)
	case "remote-herdr":
		err = fixtureRemoteHerdr(args)
	case "fake-ssh":
		err = fixtureRemoteSSH(args)
	case "ssh":
		config := os.Getenv("FIXTURE_SSH_CONFIG")
		if config == "" {
			return 23
		}
		forward := []string{"/usr/bin/ssh", "-F", config}
		for i := 0; i < len(args); i++ {
			if args[i] == "-F" && i+1 < len(args) {
				i++
				continue
			}
			forward = append(forward, args[i])
		}
		if err := appendFile(os.Getenv("FIXTURE_SSH_LOG"), []byte(fmt.Sprintf("%q\n", forward))); err != nil {
			return 23
		}
		env := slices.DeleteFunc(os.Environ(), func(s string) bool { return strings.HasPrefix(s, "HERDRCTX_TEST_HELPER=") })
		// #nosec G204 G702 -- Test-only wrapper forwards to the fixed system SSH with an isolated config.
		err = syscall.Exec("/usr/bin/ssh", forward, env)
	case "herdr":
		err = fixtureHerdr(args)
	case "curl":
		err = fixtureCurl(args)
	case "uname":
		switch {
		case slices.Equal(args, []string{"-s"}):
			fmt.Println(os.Getenv("FIXTURE_OS"))
		case slices.Equal(args, []string{"-m"}):
			fmt.Println(os.Getenv("FIXTURE_ARCH"))
		default:
			err = fmt.Errorf("unexpected uname arguments %q", args)
		}
	default:
		err = fmt.Errorf("unknown fixture %q", mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 22
	}
	return 0
}

func fixtureHerdr(args []string) error {
	if slices.Equal(args, []string{"--version"}) {
		fmt.Println("herdr 0.6.5")
		return nil
	}
	root := os.Getenv("HERDRCTX_TEST_ROOT")
	// #nosec G304 G703 -- The parent test supplies this isolated fixture directory.
	raw, err := os.ReadFile(filepath.Join(root, "sessions.json"))
	if err != nil {
		return err
	}
	var state sessionList
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if slices.Equal(args, []string{"session", "list", "--json"}) {
		if state.Fail {
			return fmt.Errorf(`{"error":{"code":"fixture_offline","message":"navigation fixture offline"}}`)
		}
		_, err := os.Stdout.Write(raw)
		return err
	}
	if len(args) == 4 && args[0] == "--session" && args[2] == "agent" && args[3] == "list" {
		for _, item := range state.Sessions {
			if item.Name == args[1] && item.Running {
				if item.Name == "running" {
					title := state.AgentTitle
					if title == "" {
						title = "Approve picker changes"
					}
					targets := state.AgentTargets
					if len(targets) == 0 {
						targets = []string{"w1:p2"}
					}
					var agents []map[string]string
					for _, target := range targets {
						agents = append(agents, map[string]string{
							"name": "review", "agent": "codex", "agent_status": "blocked",
							"foreground_cwd": "/projects/herdrctx", "title": title,
							"workspace_id": "w1", "pane_id": target,
						})
					}
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"result": map[string]any{"agents": agents},
					})
				} else {
					fmt.Println(`{"result":{"agents":[]}}`)
				}
				return nil
			}
		}
		return fmt.Errorf("agent inspection attempted for stopped or missing session %q", args[1])
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	action, err := json.Marshal(invocation{Args: args, CWD: cwd})
	if err != nil {
		return err
	}
	if err := appendFile(filepath.Join(root, "actions.jsonl"), append(action, '\n')); err != nil {
		return err
	}
	if len(args) == 3 && slices.Equal(args[:2], []string{"session", "attach"}) {
		if len(state.AgentTargets) > 0 {
			target := state.AgentTargets[0]
			// #nosec G304 G703 -- Read only the fixed focus file in the isolated fixture root.
			focused, err := os.ReadFile(filepath.Join(root, "focused-agent.txt"))
			if err == nil {
				target = string(focused)
			} else if !os.IsNotExist(err) {
				return err
			}
			fmt.Println("NAVIGATION_AGENT_VISIBLE " + target)
		}
		fmt.Println("NAVIGATION_ATTACH_HANDOFF")
		return nil
	}
	if len(args) == 5 && args[0] == "--session" && args[2] == "agent" && args[3] == "focus" {
		if len(state.AgentTargets) > 0 {
			// #nosec G703 -- Write only the fixed focus file in the isolated fixture root.
			if err := os.WriteFile(filepath.Join(root, "focused-agent.txt"), []byte(args[4]), 0o600); err != nil {
				return err
			}
		}
		fmt.Println("NAVIGATION_AGENT_FOCUSED")
		return nil
	}
	return fmt.Errorf("unexpected session action: %q", args)
}

func fixtureCurl(args []string) error {
	i := slices.Index(args, "-o")
	if i < 1 || i+1 >= len(args) {
		return fmt.Errorf("unexpected curl arguments %q", args)
	}
	url, dest := args[i-1], args[i+1]
	if err := appendFile(os.Getenv("FIXTURE_LOG"), []byte(url+"\n")); err != nil {
		return err
	}
	const prefix = "https://github.com/j0urneyk/herdrctx/releases/download/"
	if !strings.HasPrefix(url, prefix) {
		return fmt.Errorf("unexpected download URL %q", url)
	}
	if os.Getenv("FIXTURE_FAIL") != "" {
		return fmt.Errorf("injected download failure")
	}
	source := filepath.Join(os.Getenv("FIXTURE_DIR"), strings.TrimPrefix(url, prefix))
	return copyFile(source, dest, 0o600)
}
