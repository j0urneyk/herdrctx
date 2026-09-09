//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func (s *scenario) sessions(herdr string) ([]session, error) {
	raw, err := s.command(10*time.Second, herdr, "session", "list", "--json")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(s.artifacts, "sessions.json"), []byte(raw), 0o600); err != nil {
		return nil, err
	}
	var result sessionList
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	for _, entry := range result.Sessions {
		for _, path := range []string{entry.SessionDir, entry.SocketPath} {
			if !contained(s.root, path) {
				return nil, fmt.Errorf("non-isolated session path %q", path)
			}
		}
	}
	return result.Sessions, nil
}

func (s *scenario) session(herdr, name string) (session, bool, error) {
	entries, err := s.sessions(herdr)
	if err != nil {
		return session{}, false, err
	}
	for _, entry := range entries {
		if entry.Name == name {
			return entry, true, nil
		}
	}
	return session{}, false, nil
}

func TestLifecycle(t *testing.T) {
	s := newScenario(t, "lifecycle")
	s.env["PATH"] = "/usr/bin:/bin"
	s.env["PS1"] = "hctx-shell> "
	herdr := s.herdr("0.6.5", *herdrFlag)
	name := "ci-" + filepath.Base(s.root)
	s.metadata["session"] = name
	s.metadata["terminal"] = "xterm-256color, 160x40"
	shell := filepath.Join(s.root, "test-shell")
	writeFile(t, shell, []byte("#!/bin/sh\nexport PS1='hctx-shell> '\nexec /bin/sh\n"), 0o700)
	writeFile(t, s.env["HERDR_CONFIG_PATH"], []byte(fmt.Sprintf("onboarding = false\n[terminal]\ndefault_shell = %q\nnew_cwd = \"current\"\n[ui.sound]\nenabled = false\n", shell)), 0o600)
	_, err := s.sessions(herdr)
	must(t, err)
	pid := 0
	pidFile := filepath.Join(s.root, "work", "pane.pid")
	t.Cleanup(func() {
		if t.Failed() {
			s.snapshot()
		}
		state, exists, cleanupErr := s.session(herdr, name)
		if exists {
			if state.Running {
				_, err := s.command(10*time.Second, herdr, "session", "stop", name, "--json")
				cleanupErr = errors.Join(cleanupErr, err)
			}
			_, err := s.command(10*time.Second, herdr, "session", "delete", name, "--json")
			cleanupErr = errors.Join(cleanupErr, err)
		}
		_, exists, err := s.session(herdr, name)
		cleanupErr = errors.Join(cleanupErr, err)
		removed := err == nil && !exists
		if !removed {
			cleanupErr = errors.Join(cleanupErr, errors.New("test session remains after cleanup"))
		}
		if pid == 0 {
			// #nosec G304 -- Read only the PID file written by this test's isolated shell.
			if raw, err := os.ReadFile(pidFile); err == nil {
				pid, err = strconv.Atoi(strings.TrimSpace(string(raw)))
				cleanupErr = errors.Join(cleanupErr, err)
			} else if !os.IsNotExist(err) {
				cleanupErr = errors.Join(cleanupErr, err)
			}
		}
		var processErr error
		shellExited := pid == 0
		err = waitFor(context.Background(), 5*time.Second, func() bool {
			var alive bool
			alive, processErr = processRunning(pid)
			shellExited = !alive && processErr == nil
			return shellExited || processErr != nil
		})
		cleanupErr = errors.Join(cleanupErr, err, processErr)
		s.writeJSON("cleanup.json", map[string]any{"session": name, "session_removed": removed, "pane_pid": pid, "shell_exited": shellExited})
		if cleanupErr != nil {
			s.writeJSON("cleanup-error.json", map[string]string{"error": cleanupErr.Error()})
			t.Errorf("lifecycle cleanup: %v", cleanupErr)
		}
	})
	running := func(want bool) bool {
		state, exists, err := s.session(herdr, name)
		must(t, err)
		return exists && state.Running == want
	}

	p := s.terminal("lifecycle", repoPath(*binaryFlag), "--herdr-bin", herdr, "--interval", "500ms")
	p.expect("Loaded 1 session(s).", 0)
	s.check("create and attach immediately through n")
	offset := p.offset()
	p.send("n")
	p.expect("Enter to create and attach.", offset)
	p.send(name + enter)
	s.wait("running session", func() bool { return running(true) })
	p.expect("No workspaces yet", offset)
	p.send("\x02N")
	p.expect("hctx-shell>", offset)
	panes := decodeJSON[struct {
		Result struct {
			Panes []struct {
				ID string `json:"pane_id"`
			} `json:"panes"`
		} `json:"result"`
	}](t, s.cli(herdr, "--session", name, "pane", "list"))
	if len(panes.Result.Panes) != 1 {
		t.Fatalf("expected one shell pane: %+v", panes)
	}
	paneID := panes.Result.Panes[0].ID
	shellContains := func(marker string) bool {
		raw := s.cli(herdr, "--session", name, "pane", "read", paneID, "--source", "recent-unwrapped", "--lines", "100", "--format", "text")
		writeFile(t, filepath.Join(s.artifacts, "pane.txt"), []byte(raw), 0o600)
		return strings.Contains(raw, marker)
	}
	s.check("detach and reattach preserve shell process and output")
	for visit := 1; visit <= 2; visit++ {
		marker := fmt.Sprintf("HCTX_%d_%d", time.Now().UnixNano(), visit)
		// Split the marker so the typed command's echo cannot satisfy the output assertion.
		command := "printf '%s\\n' \"$$\" > pane.pid; printf '%s%s\\n' " + quote(marker[:12]) + " " + quote(marker[12:])
		p.send("\x1b[200~" + command + "\x1b[201~")
		s.wait("complete pasted shell command", func() bool { return shellContains(command) })
		p.send(enter)
		s.wait("shell output "+marker, func() bool { return shellContains(marker) })
		s.wait("shell PID", func() bool { _, err := os.Stat(pidFile); return err == nil })
		current, err := strconv.Atoi(strings.TrimSpace(string(readFile(t, pidFile))))
		must(t, err)
		if pid != 0 && current != pid {
			t.Fatal("reattach replaced the shell process")
		}
		pid = current
		if *failAfterAttachFlag && visit == 1 {
			t.Fatal("Injected failure after attach")
		}
		offset = p.offset()
		p.send("\x02q")
		p.expect("herdrctx", offset)
		p.expect(name, offset)
		s.wait("session survives detach", func() bool { return running(true) })
		if visit == 1 {
			offset = p.offset()
			p.send("/" + name + enter)
			p.expect(name, offset)
			p.send(enter)
			p.expect("hctx-shell>", offset)
			s.wait("preserved shell output", func() bool { return shellContains(marker) })
		}
	}

	s.check("stop requires confirmation and stopped attachment stays blocked")
	offset = p.offset()
	p.send("s")
	p.expect(fmt.Sprintf("Stop session %q?", name), offset)
	if !running(true) {
		t.Fatal("session stopped before confirmation")
	}
	p.send(enter)
	s.wait("stopped session", func() bool { return running(false) })
	s.wait("shell exit", func() bool { alive, err := processRunning(pid); must(t, err); return !alive })
	p.expect("stopped", offset)
	p.sendExpect(enter, "Stopped session attach disabled")
	if !running(false) {
		t.Fatal("blocked attachment restarted the session")
	}
	p.sendExpect(enter, "attach disabled")
	s.check("delete requires confirmation and quit exits normally")
	offset = p.offset()
	p.send("d")
	p.expect(fmt.Sprintf("Delete session %q?", name), offset)
	if !running(false) {
		t.Fatal("session deleted before confirmation")
	}
	p.send(enter)
	s.wait("deleted session", func() bool { _, exists, err := s.session(herdr, name); must(t, err); return !exists })
	p.expect("No sessions match", offset)
	p.quit()
}
