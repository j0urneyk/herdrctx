//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j0urneyk/herdrctx/internal/herdr"
)

func TestAgentPickerRealSessionFocus(t *testing.T) {
	for _, version := range []string{"0.6.5", "0.8.2", "0.9.0"} {
		t.Run(version, func(t *testing.T) {
			s := newScenario(t, "agent-focus-"+version)
			s.env["PATH"] = "/usr/bin:/bin"
			bin := s.herdr(version, "")
			name := "agent-picker"
			shell := filepath.Join(s.root, "agent-shell")
			writeFile(t, shell, []byte("#!/bin/sh\nprintf '%s\\n' \"$$\" > .shell.pid\nprintf 'HCTX_SELECTED_%s_READY\\n' \"${PWD##*/}\"\nexport PS1='agent-shell> '\nexec /bin/sh\n"), 0o700)
			writeFile(t, s.env["HERDR_CONFIG_PATH"], []byte(fmt.Sprintf("onboarding = false\n[terminal]\ndefault_shell = %q\n[ui.sound]\nenabled = false\n", shell)), 0o600)
			var paneIDs []string
			var shellPIDs []int
			t.Cleanup(func() {
				state, exists, cleanupErr := s.session(bin, name)
				if exists && state.Running {
					_, err := s.command(10*time.Second, bin, "session", "stop", name, "--json")
					cleanupErr = errors.Join(cleanupErr, err)
				}
				if exists {
					_, err := s.command(10*time.Second, bin, "session", "delete", name, "--json")
					cleanupErr = errors.Join(cleanupErr, err)
				}
				_, exists, err := s.session(bin, name)
				cleanupErr = errors.Join(cleanupErr, err)
				if exists {
					cleanupErr = errors.Join(cleanupErr, errors.New("agent test session remains"))
				}
				shellsExited := true
				for _, pid := range shellPIDs {
					err := waitFor(context.Background(), 5*time.Second, func() bool {
						alive, err := processRunning(pid)
						return err == nil && !alive
					})
					shellsExited = shellsExited && err == nil
					cleanupErr = errors.Join(cleanupErr, err)
				}
				s.writeJSON("cleanup.json", map[string]any{"session_removed": !exists, "shells_exited": shellsExited})
				if cleanupErr != nil {
					t.Errorf("agent focus cleanup: %v", cleanupErr)
				}
			})
			setup := s.terminal("setup", bin, "--session", name)
			if version == "0.6.5" {
				setup.expect("No workspaces yet", 0)
			} else {
				setup.expect("agent-shell>", 0)
				pid, err := strconv.Atoi(strings.TrimSpace(string(readFile(t, filepath.Join(s.root, "work", ".shell.pid")))))
				must(t, err)
				shellPIDs = append(shellPIDs, pid)
			}
			type paneList struct {
				Result struct {
					Panes []struct {
						ID string `json:"pane_id"`
					} `json:"panes"`
				} `json:"result"`
			}
			initial := decodeJSON[paneList](t, s.cli(bin, "--session", name, "pane", "list"))
			knownPanes := make(map[string]bool)
			for _, pane := range initial.Result.Panes {
				knownPanes[pane.ID] = true
			}
			for i := 1; i <= 3; i++ {
				dir := filepath.Join(s.root, "work", fmt.Sprintf("pane-%d", i))
				must(t, os.MkdirAll(dir, 0o700))
				s.cli(bin, "--session", name, "workspace", "create", "--cwd", dir, "--label", fmt.Sprintf("Workspace %d", i))
				panes := decodeJSON[paneList](t, s.cli(bin, "--session", name, "pane", "list"))
				id := ""
				for _, pane := range panes.Result.Panes {
					if !knownPanes[pane.ID] {
						id = pane.ID
						knownPanes[pane.ID] = true
					}
				}
				if id == "" {
					t.Fatal("workspace did not create a new pane")
				}
				paneIDs = append(paneIDs, id)
				s.cli(bin, "--session", name, "pane", "report-agent", id, "--source", "herdrctx-integration", "--agent", "codex", "--state", "idle")
				s.cli(bin, "--session", name, "agent", "rename", id, fmt.Sprintf("agent-%d", i))
				marker := fmt.Sprintf("HCTX_SELECTED_pane-%d_READY", i)
				s.wait("agent shell ready", func() bool {
					raw := s.cli(bin, "--session", name, "pane", "read", id, "--source", "recent-unwrapped", "--lines", "100", "--format", "text")
					return strings.Contains(raw, marker)
				})
				pid, err := strconv.Atoi(strings.TrimSpace(string(readFile(t, filepath.Join(dir, ".shell.pid")))))
				must(t, err)
				shellPIDs = append(shellPIDs, pid)
			}
			s.cli(bin, "--session", name, "agent", "focus", paneIDs[0])
			setup.send("\x02q")
			setup.waitExit()
			agents, err := herdr.ParseAgents([]byte(s.cli(bin, "--session", name, "agent", "list")))
			must(t, err)
			if len(agents) != 3 {
				t.Fatalf("agents = %#v", agents)
			}
			p := s.terminal("picker", repoPath(*binaryFlag), "--herdr-bin", bin, "--interval", "1h")
			p.expect("agent-3", 0)
			p.send("/" + name + enter)
			for _, target := range []int{2, 1, -1} {
				s.cli(bin, "--session", name, "agent", "focus", paneIDs[0])
				row, expectedPane := 0, 1
				if target >= 0 {
					expectedPane = target + 1
					for i, agent := range agents {
						if agent.Target == paneIDs[target] {
							row = i + 1
						}
					}
					if row == 0 {
						t.Fatal("target missing from agent rows")
					}
				}
				offset := p.offset()
				p.send(strings.Repeat(up, len(agents)+1) + strings.Repeat(down, row) + enter)
				p.expect(fmt.Sprintf("HCTX_SELECTED_pane-%d_READY", expectedPane), offset)
				offset = p.offset()
				p.send("\x02q")
				p.expect("enter/a", offset)
			}
			p.send("q")
			p.waitExit()
			s.check("agent rows open the chosen workspace immediately; parent attachment preserves its current workspace")
		})
	}
}
