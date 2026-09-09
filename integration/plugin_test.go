//go:build integration

package integration

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPluginRegistration(t *testing.T) { testPlugin(t, "") }

func TestPublicPlugin(t *testing.T) {
	if *sourceRefFlag == "" {
		t.Skip("set -integration.source-ref to test a reviewed, published Git ref")
	}
	testPlugin(t, *sourceRefFlag)
}

func testPlugin(t *testing.T, sourceRef string) {
	t.Helper()
	s := newScenario(t, "plugin")
	herdr := s.herdr("0.7.0", *pluginHerdrFlag)
	s.env["HERDRCTX_INSTALL_DIR"] = filepath.Join(s.root, "bin")
	installed := filepath.Join(s.root, "bin", "herdrctx")
	writeFile(t, s.env["HERDR_CONFIG_PATH"], []byte("onboarding = false\n[terminal]\ndefault_shell = \"/bin/sh\"\n[ui.sound]\nenabled = false\n"), 0o600)
	entries, err := s.sessions(herdr)
	must(t, err)
	for _, entry := range entries {
		if entry.Running {
			t.Fatal("isolated session already running")
		}
	}
	plugins := func() []map[string]any {
		result := decodeJSON[struct {
			Result struct{ Plugins []map[string]any }
		}](t, s.cli(herdr, "plugin", "list", "--json"))
		return result.Result.Plugins
	}
	if len(plugins()) != 0 {
		t.Fatal("isolated registry is not empty")
	}
	if sourceRef != "" {
		s.check("install a reviewed public ref without starting a server")
		output, err := s.command(5*time.Minute, herdr, "plugin", "install", "j0urneyk/herdrctx", "--ref", sourceRef, "--yes")
		if err != nil {
			t.Fatalf("public install: %v\n%s", err, output)
		}
		if _, err := os.Stat(installed); err != nil {
			t.Fatal("build command did not install the binary")
		}
		entries, err = s.sessions(herdr)
		must(t, err)
		for _, entry := range entries {
			if entry.Running {
				t.Fatal("installation started a server")
			}
		}
	} else {
		s.startPluginServer(herdr)
		s.check("local linking registers without running the installer")
		s.cli(herdr, "plugin", "link", repo)
		if _, err := os.Stat(installed); !os.IsNotExist(err) {
			t.Fatal("plugin link unexpectedly ran the installer")
		}
		must(t, copyFile(repoPath(*binaryFlag), installed, 0o700))
	}
	registered := plugins()
	if len(registered) != 1 {
		t.Fatalf("expected one registered plugin: %v", registered)
	}
	plugin := registered[0]
	if plugin["plugin_id"] != "herdrctx" {
		t.Fatalf("unexpected plugin: %v", plugin)
	}
	wantBuild := []any{map[string]any{"command": []any{"sh", "scripts/install.sh"}}}
	if !reflect.DeepEqual(plugin["build"], wantBuild) {
		t.Fatalf("unexpected build command: %v", plugin["build"])
	}
	for _, key := range []string{"panes", "actions", "startup", "events", "link_handlers"} {
		if value := plugin[key]; value != nil && !reflect.ValueOf(value).IsZero() {
			v := reflect.ValueOf(value)
			if (v.Kind() != reflect.Slice && v.Kind() != reflect.Map) || v.Len() != 0 {
				t.Fatalf("unexpected %s: %v", key, value)
			}
		}
	}
	configDir := strings.TrimSpace(s.cli(herdr, "plugin", "config-dir", "herdrctx"))
	if !contained(s.root, configDir) {
		t.Fatalf("non-isolated plugin config: %s", configDir)
	}
	if info, err := os.Stat(filepath.Join(s.root, "state", "herdr", "plugins", "herdrctx")); err != nil || !info.IsDir() {
		t.Fatal("missing plugin state directory")
	}
	installedVersion := strings.TrimSpace(s.cli(installed, "--version"))
	if sourceRef != "" && installedVersion != "herdrctx "+fmt.Sprint(plugin["version"]) {
		t.Fatalf("installed version differs from manifest: %s", installedVersion)
	}
	expectContains(t, s.cli(installed, "--help"), "Usage: herdrctx [flags]")

	s.check("both nested signals block attach and both creation shortcuts")
	entries, err = s.sessions(herdr)
	must(t, err)
	if len(entries) != 1 {
		t.Fatalf("expected one isolated default session: %v", entries)
	}
	for _, signal := range []struct{ key, value string }{{"HERDR_ENV", "1"}, {"HERDR_SOCKET_PATH", entries[0].SocketPath}} {
		s.env[signal.key] = signal.value
		for _, action := range []struct{ key, label, title string }{
			{"a", "attach", "Cannot attach from inside Herdr"},
			{"n", "create", "Cannot create from inside Herdr"},
			{"N", "create-directory", "Cannot create from inside Herdr"},
		} {
			p := s.terminal("nested-"+signal.key+"-"+action.label, installed, "--herdr-bin", herdr)
			p.expect("Loaded 1 session(s).", 0)
			p.sendExpect(action.key, action.title)
			p.send(enter)
			p.quit()
		}
		delete(s.env, signal.key)
	}

	s.check("uninstall removes registration and preserves the external binary")
	before := readFile(t, installed)
	managedRoot, ok := plugin["plugin_root"].(string)
	if !ok {
		t.Fatal("missing plugin root")
	}
	if sourceRef != "" && !contained(s.root, managedRoot) {
		t.Fatalf("non-isolated managed checkout: %s", managedRoot)
	}
	s.cli(herdr, "plugin", "uninstall", "herdrctx")
	if len(plugins()) != 0 {
		t.Fatal("plugin remains registered")
	}
	if !bytes.Equal(readFile(t, installed), before) {
		t.Fatal("uninstall changed the external binary")
	}
	if sourceRef != "" {
		if _, err := os.Stat(managedRoot); !os.IsNotExist(err) {
			t.Fatal("managed checkout remains after uninstall")
		}
	} else {
		if _, err := os.Stat(filepath.Join(repo, "herdr-plugin.toml")); err != nil {
			t.Fatal("uninstall removed the local checkout")
		}
	}
	s.metadata["plugin"] = plugin
	s.metadata["installed_version"] = installedVersion
	s.metadata["source_ref"] = sourceRef
}

func (s *scenario) startPluginServer(herdr string) {
	s.t.Helper()
	log, err := os.Create(filepath.Join(s.artifacts, "server.log"))
	must(s.t, err)
	cmd := exec.Command(herdr, "server")
	cmd.Dir, cmd.Env = s.root, environment(s.env)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = log.Close()
		s.t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	s.t.Cleanup(func() {
		var cleanupErr error
		select {
		case <-done:
			cleanupErr = waitErr
		default:
			_, cleanupErr = s.command(10*time.Second, herdr, "server", "stop")
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				cleanupErr = errors.Join(cleanupErr, errors.New("server did not stop"), killGroup(cmd.Process.Pid, syscall.SIGKILL))
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					cleanupErr = errors.Join(cleanupErr, errors.New("server did not exit after SIGKILL"))
				}
			}
		}
		cleanupErr = errors.Join(cleanupErr, log.Close())
		entries, err := s.sessions(herdr)
		cleanupErr = errors.Join(cleanupErr, err)
		stopped := err == nil
		for _, entry := range entries {
			stopped = stopped && !entry.Running
		}
		if !stopped {
			cleanupErr = errors.Join(cleanupErr, errors.New("isolated server remains running"))
		}
		s.writeJSON("cleanup.json", map[string]any{"server_stopped": stopped})
		if cleanupErr != nil {
			s.t.Errorf("plugin server cleanup: %v", cleanupErr)
		}
	})
	s.wait("isolated plugin server readiness", func() bool {
		select {
		case <-done:
			s.t.Fatalf("plugin server exited: %v", waitErr)
		default:
		}
		entries, err := s.sessions(herdr)
		must(s.t, err)
		for _, entry := range entries {
			if entry.Running {
				return true
			}
		}
		return false
	})
}
