//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j0urneyk/herdrctx/internal/preferences"
)

var remoteFlag = flag.Bool("integration.remote", false, "run the opt-in local Docker SSH tests")

type remoteFixture struct {
	s         *scenario
	container string
	herdr     string
	ssh       string
	target    string
	proxy     *remoteProxy
}

func newRemoteFixture(t *testing.T, version string) *remoteFixture {
	t.Helper()
	s := newScenario(t, "remote-"+version)
	secrets := filepath.Join(s.root, "secrets")
	key := filepath.Join(secrets, "id_ed25519")
	hostKey := filepath.Join(secrets, "host_ed25519")
	t.Cleanup(func() {
		s.snapshot()
		artifactRoot, err := os.OpenRoot(s.artifacts)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = artifactRoot.Close() }()
		for _, keyPath := range []string{key, hostKey} {
			// #nosec G304 -- Both paths are ephemeral keys created by this fixture.
			private, readErr := os.ReadFile(keyPath)
			if os.IsNotExist(readErr) {
				continue
			}
			if readErr != nil {
				t.Error(readErr)
				continue
			}
			err := filepath.WalkDir(s.artifacts, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.Type().IsRegular() {
					return nil
				}
				rel, err := filepath.Rel(s.artifacts, path)
				if err != nil {
					return err
				}
				raw, err := artifactRoot.ReadFile(rel)
				if err != nil {
					return err
				}
				if bytes.Contains(raw, private) || containsPrivateKey(raw) {
					return fmt.Errorf("private key in diagnostic artifact %s", path)
				}
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}
	})
	cmd := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
	raw, err := cmd.Output()
	must(t, err)
	endpoint := strings.TrimSpace(string(raw))
	if override := os.Getenv("DOCKER_HOST"); override != "" {
		endpoint = override
	}
	if !strings.HasPrefix(endpoint, "unix://") {
		t.Fatal("remote tests require a local Unix-socket Docker engine")
	}
	s.env["DOCKER_HOST"] = endpoint
	name := "herdrctx-" + filepath.Base(s.root)
	f := &remoteFixture{s: s, container: name, target: "hctx-test"}
	containerID := ""
	t.Cleanup(func() {
		if containerID == "" {
			return
		}
		logs, _ := s.command(5*time.Second, "docker", "logs", containerID)
		writeFile(t, filepath.Join(s.artifacts, "sshd.log"), []byte(logs), 0o600)
		_, err := s.command(15*time.Second, "docker", "rm", "--force", containerID)
		s.writeJSON("cleanup.json", map[string]any{"container": name, "container_id": containerID, "removed": err == nil})
		if err != nil {
			t.Errorf("remove test container: %v", err)
		}
	})
	must(t, os.MkdirAll(secrets, 0o700))
	s.cli("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key)
	s.cli("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", hostKey)
	image := "herdrctx-remote-test:" + version
	s.metadata["remote_image"] = strings.TrimSpace(s.cli("docker", "image", "inspect", "--format", "{{.Id}}", image))
	imageArch := strings.TrimSpace(s.cli("docker", "image", "inspect", "--format", "{{.Architecture}}", image))
	s.metadata["remote_platform"] = "linux/" + imageArch
	containerID = strings.TrimSpace(s.cli("docker", "create", "--name", name, "--label", "herdrctx.test=true", "--publish", "127.0.0.1::22", image))
	f.container = containerID
	s.cli("docker", "cp", key+".pub", name+":/home/tester/.ssh/authorized_keys")
	s.cli("docker", "cp", hostKey, name+":/etc/ssh/ssh_host_ed25519_key")
	s.cli("docker", "cp", hostKey+".pub", name+":/etc/ssh/ssh_host_ed25519_key.pub")
	s.cli("docker", "start", name)
	s.cli("docker", "exec", name, "chown", "tester:tester", "/home/tester/.ssh/authorized_keys")
	port := strings.TrimSpace(s.cli("docker", "port", name, "22/tcp"))
	if !strings.HasPrefix(port, "127.0.0.1:") {
		t.Fatalf("unexpected binding %q", port)
	}
	f.proxy = newRemoteProxy(t, port)
	_, portNumber, err := net.SplitHostPort(f.proxy.listener.Addr().String())
	must(t, err)
	known := filepath.Join(secrets, "known_hosts")
	writeFile(t, known, []byte("[127.0.0.1]:"+portNumber+" "+string(readFile(t, hostKey+".pub"))), 0o600)
	config := filepath.Join(secrets, "config")
	writeFile(t, config, []byte(fmt.Sprintf("Host *\n  HostName 127.0.0.1\n  Port %s\n  User tester\n  IdentityFile %s\n  IdentitiesOnly yes\n  IdentityAgent none\n  UserKnownHostsFile %s\n  GlobalKnownHostsFile /dev/null\n  StrictHostKeyChecking yes\n  BatchMode yes\n  ConnectTimeout 3\n  ControlMaster no\n  ControlPath none\n", portNumber, key, known)), 0o600)
	s.env["FIXTURE_SSH_CONFIG"] = config
	s.env["FIXTURE_SSH_LOG"] = filepath.Join(s.artifacts, "ssh-args.log")
	f.ssh = s.fixture("ssh")
	s.env["PATH"] = filepath.Dir(f.ssh) + string(os.PathListSeparator) + s.env["PATH"]
	s.wait("SSH ready", func() bool { _, err := s.command(4*time.Second, f.ssh, f.target, "true"); return err == nil })
	effective := s.cli(f.ssh, "-G", f.target)
	for _, setting := range []string{"hostname 127.0.0.1", "user tester", "port " + portNumber, "identityagent none", "userknownhostsfile " + known} {
		if !strings.Contains(effective, setting) {
			t.Fatalf("SSH setting not applied: %s", setting)
		}
	}
	f.herdr = s.herdr(version, "")
	f.cli("printf %s " + quote("#!/bin/sh\nPS1='hctx-remote> '; export PS1\nexec /bin/sh\n") + " > /home/tester/test-shell; chmod 700 /home/tester/test-shell")
	remoteConfig := "onboarding = false\n[terminal]\ndefault_shell = \"/home/tester/test-shell\"\nnew_cwd = \"current\"\n[ui.sound]\nenabled = false\n"
	localConfig := remoteConfig + "[remote]\nmanage_ssh_config = false\n"
	writeFile(t, s.env["HERDR_CONFIG_PATH"], []byte(localConfig), 0o600)
	f.cli("mkdir -p ~/.config/herdr; printf %s " + quote(remoteConfig) + " > ~/.config/herdr/config.toml")
	remoteVersion := strings.TrimSpace(f.cli("herdr --version"))
	if remoteVersion != "herdr "+version {
		t.Fatalf("unexpected remote version: %s", remoteVersion)
	}
	s.metadata["remote_version"] = remoteVersion
	remoteDigest := strings.Fields(s.cli("docker", "exec", name, "sha256sum", "/usr/local/bin/herdr"))[0]
	if remoteDigest != testDigests[version]["linux/"+imageArch] {
		t.Fatal("remote Herdr checksum mismatch")
	}
	s.metadata["remote_sha256"] = remoteDigest
	s.metadata["ssh_target"] = port

	return f
}

func (f *remoteFixture) cli(command string) string { return f.s.cli(f.ssh, f.target, command) }

func (f *remoteFixture) sessions() []session {
	var result sessionList
	raw := f.cli("herdr session list --json")
	must(f.s.t, json.Unmarshal([]byte(raw), &result))
	writeFile(f.s.t, filepath.Join(f.s.artifacts, "sessions.json"), []byte(raw), 0o600)
	for _, s := range result.Sessions {
		if !strings.HasPrefix(s.SessionDir, "/home/tester/") || !strings.HasPrefix(s.SocketPath, "/home/tester/") {
			f.s.t.Fatalf("non-isolated remote session: %+v", s)
		}
	}
	return result.Sessions
}

func (f *remoteFixture) running(name string) bool {
	for _, s := range f.sessions() {
		if s.Name == name {
			return s.Running
		}
	}
	return false
}

func (f *remoteFixture) waitShellExit(pid string) {
	f.s.wait("remote shell exited", func() bool {
		state := strings.TrimSpace(f.cli("ps -o stat= -p " + pid + " || true"))
		return state == "" || strings.HasPrefix(state, "Z")
	})
}

func TestRemoteContract(t *testing.T) {
	if !*remoteFlag {
		t.Skip("enable with -integration.remote")
	}
	for _, version := range []string{"0.8.2", "0.9.0"} {
		t.Run(version, func(t *testing.T) {
			f := newRemoteFixture(t, version)
			s := f.s
			name := "contract"
			f.sessions()
			p := s.terminal("native", f.herdr, "--remote", f.target, "--session", name)
			s.wait("remote running", func() bool { return f.running(name) })
			p.expect("hctx-remote>", 0)
			p.send("printf '%s\\n' \"$$\" > /home/tester/pane.pid; pwd > /home/tester/pane.cwd\r")
			s.wait("remote shell", func() bool {
				_, err := s.command(5*time.Second, f.ssh, f.target, "test -s /home/tester/pane.pid")
				return err == nil
			})
			pid := strings.TrimSpace(f.cli("cat /home/tester/pane.pid"))
			cwd := strings.TrimSpace(f.cli("cat /home/tester/pane.cwd"))
			s.metadata["initial_cwd"] = cwd
			p.send("\x02q")
			p.waitExit()
			p = s.terminal("reattach", f.herdr, "--remote", f.target, "--session", name)
			p.expect("hctx-remote>", 0)
			if strings.TrimSpace(f.cli("cat /home/tester/pane.pid")) != pid {
				t.Fatal("shell changed")
			}
			f.cli("kill -0 " + pid)
			p.send("\x02q")
			p.waitExit()
			f.cli("herdr session stop " + name + " --json")
			f.waitShellExit(pid)
			if f.running(name) {
				t.Fatal("session still running")
			}
			p = s.terminal("restart", f.herdr, "--remote", f.target, "--session", name)
			s.wait("restarted", func() bool { return f.running(name) })
			p.expect("hctx-remote>", 0)
			p.send("\x02q")
			p.waitExit()
			f.cli("herdr session stop " + name + " --json")
			f.cli("herdr session delete " + name + " --json")
			for _, entry := range f.sessions() {
				if entry.Name == name {
					t.Fatal("session remains")
				}
			}
			s.check("native remote create/detach/reattach/stop/restart/delete")
		})
	}
}

func TestRemoteLifecycle(t *testing.T) {
	if !*remoteFlag {
		t.Skip("enable with -integration.remote")
	}
	for _, version := range []string{"0.8.2", "0.9.0"} {
		t.Run(version, func(t *testing.T) { testRemoteLifecycle(t, version) })
	}
}

func testRemoteLifecycle(t *testing.T, version string) {
	f := newRemoteFixture(t, version)
	f.useVersionClient(version)
	delete(f.s.env, "HERDR_REMOTE_BINARY")
	s := f.s
	name := "remote-api"
	p := s.terminal("herdrctx-remote", repoPath(*binaryFlag), "--herdr-bin", f.herdr, "--remote", f.target, "--interval", "500ms")
	p.expect("Loaded 1 session(s).", 0)
	p.sendExpect("d", "Default session cannot be deleted")
	p.send("ysn")
	defaultPresent := false
	for _, entry := range f.sessions() {
		if entry.Name == "default" && entry.Default {
			defaultPresent = true
		}
	}
	if !defaultPresent {
		t.Fatal("default session was removed or lost its default flag")
	}
	sshLog := readFile(t, s.env["FIXTURE_SSH_LOG"])
	if strings.Contains(string(sshLog), "session delete") {
		t.Fatal("default delete reached SSH")
	}
	p.sendExpect(enter, "enter/a")
	s.check("default session deletion is blocked before invoking the remote CLI")
	p.sendExpect("N", "Remote directory creation unavailable")
	p.send(enter)
	p.sendExpect("n", "Uses the remote default directory.")
	p.send(name + enter)
	p.expect("hctx-remote>", 0)
	s.wait("created remote session", func() bool { return f.running(name) })
	pid := ""
	marker := "HCTX_REMOTE_PERSISTENT_OUTPUT"
	for visit := 0; visit < 2; visit++ {
		f.cli("rm -f /home/tester/visit.pid")
		p.send("printf '%s\\n' \"$$\" > /home/tester/visit.pid; printf '%s%s\\n' HCTX_REMOTE_ PERSISTENT_OUTPUT\r")
		s.wait("shell command ran", func() bool {
			_, err := s.command(5*time.Second, f.ssh, f.target, "test -s /home/tester/visit.pid")
			return err == nil
		})
		current := strings.TrimSpace(f.cli("cat /home/tester/visit.pid"))
		if pid != "" && current != pid {
			t.Fatalf("shell changed from %s to %s", pid, current)
		}
		pid = current
		f.recordVersionServer(version, pid, "/usr/local/bin/herdr")
		if *failAfterAttachFlag {
			t.Fatal("Injected failure after remote attach")
		}
		offset := p.offset()
		p.send("\x02q")
		p.expect("herdrctx", offset)
		p.expect(name, offset)
		p.expect("Loaded 2 session(s).", offset)
		if visit == 0 {
			p.send("/" + name + enter)
			offset = p.offset()
			p.send(enter)
			p.expect("hctx-remote>", offset)
			p.expect(marker, offset)
		}
	}
	s.check("remote n and attach preserve the shell through detach")
	p.send("p")
	s.wait("remote favorite saved", func() bool {
		d, err := (&preferences.Store{Path: s.preferencesPath()}).Load()
		return err == nil && len(d.RemoteFavorites) == 1 && d.RemoteFavorites[0].Target == f.target && d.RemoteFavorites[0].Name == name
	})
	p.sendExpect("i", "Host: "+f.target)
	p.send(enter)
	p.sendExpect("s", fmt.Sprintf("Stop session %q on %s?", name, f.target))
	if !f.running(name) {
		t.Fatal("stopped without confirmation")
	}
	p.send("n")
	if !f.running(name) {
		t.Fatal("stop cancellation ignored")
	}
	s.check("remote favorite, details, and stop confirmation")
	offset := p.offset()
	f.proxy.setOffline(true)
	p.expect("Refresh failed", offset)
	p.sendExpect(enter, "Remote check failed")
	offset = p.offset()
	f.proxy.setOffline(false)
	p.send(enter)
	p.send("r")
	s.wait("SSH recovered", func() bool { return f.running(name) })
	p.expect("Loaded 2 session(s).", offset)
	p.sendExpect("s", fmt.Sprintf("Stop session %q on %s?", name, f.target))
	offset = p.offset()
	p.send(enter)
	s.wait("remote stopped", func() bool { return !f.running(name) })
	f.waitShellExit(pid)
	p.expect("stopped", offset)
	p.sendExpect(enter, "Stopped session attach disabled")
	p.send(enter)
	p.sendExpect("d", fmt.Sprintf("Delete session %q on %s?", name, f.target))
	p.send(enter)
	s.wait("remote deleted", func() bool {
		for _, entry := range f.sessions() {
			if entry.Name == name {
				return false
			}
		}
		return true
	})
	p.quit()
	s.check("network outage, recovery, stopped guard, and confirmed deletion")
}

func TestSnapshotExcludesKeysAndSymlinks(t *testing.T) {
	s := newScenario(t, "snapshot-policy")
	private := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nfixture-only\n-----END OPENSSH PRIVATE KEY-----")
	writeFile(t, filepath.Join(s.root, "secrets", "id"), private, 0o600)
	writeFile(t, filepath.Join(s.root, "work", "renamed.log"), private, 0o600)
	writeFile(t, filepath.Join(s.root, "config", "diagnostic.txt"), []byte("safe diagnostic"), 0o600)
	must(t, os.Symlink(filepath.Join(s.root, "secrets"), filepath.Join(s.root, "work", "link")))
	s.snapshot()
	for _, path := range []string{"secrets/id", "work/renamed.log", "work/link"} {
		if _, err := os.Lstat(filepath.Join(s.artifacts, "snapshot", path)); !os.IsNotExist(err) {
			t.Fatalf("included %s: %v", path, err)
		}
	}
	if string(readFile(t, filepath.Join(s.artifacts, "snapshot/config/diagnostic.txt"))) != "safe diagnostic" {
		t.Fatal("lost diagnostics")
	}
}

func TestRemoteLostMutationResponse(t *testing.T) {
	if !*remoteFlag {
		t.Skip("enable with -integration.remote")
	}
	f := newRemoteFixture(t, "0.8.2")
	s := f.s
	name := "lost-response"
	p := s.terminal("seed", f.herdr, "--remote", f.target, "--session", name)
	p.expect("hctx-remote>", 0)
	p.send("\x02q")
	p.waitExit()
	s.cli("docker", "exec", f.container, "mv", "/usr/local/bin/herdr", "/usr/local/bin/herdr-real")
	wrapper := filepath.Join(s.root, "work", "herdr-wrapper")
	writeFile(t, wrapper, []byte(`#!/bin/sh
if [ "$1" = session ] && [ "$2" = stop ]; then
  printf 'stop\n' >> /home/tester/stop-invocations
  /usr/local/bin/herdr-real "$@" > /home/tester/stop-result.json 2>&1
  touch /home/tester/stop-ready
  sleep 30
  exit 0
fi
exec /usr/local/bin/herdr-real "$@"
`), 0o755)
	s.cli("docker", "cp", wrapper, f.container+":/usr/local/bin/herdr")
	p = s.terminal("lost-response", repoPath(*binaryFlag), "--herdr-bin", f.herdr, "--remote", f.target, "--interval", "500ms")
	p.expect("Loaded 2 session(s).", 0)
	p.send("/" + name + enter)
	p.sendExpect("s", fmt.Sprintf("Stop session %q on %s?", name, f.target))
	offset := p.offset()
	p.send(enter)
	s.wait("remote mutation completed", func() bool {
		_, err := s.command(5*time.Second, "docker", "exec", f.container, "test", "-f", "/home/tester/stop-ready")
		return err == nil
	})
	f.proxy.setOffline(true)
	p.expect("Remote result unknown", offset)
	f.proxy.setOffline(false)
	p.send(enter)
	p.expect("stopped", offset)
	if count := strings.TrimSpace(f.cli("wc -l < /home/tester/stop-invocations")); count != "1" {
		t.Fatalf("mutation replayed %s times", count)
	}
	p.quit()
	s.check("completed remote stop with lost response is not replayed")
}

func TestRemoteActiveTransportLoss(t *testing.T) {
	if !*remoteFlag {
		t.Skip("enable with -integration.remote")
	}
	f := newRemoteFixture(t, "0.8.2")
	s := f.s
	name := "transport-loss"
	p := s.terminal("transport", f.herdr, "--remote", f.target, "--session", name)
	p.expect("hctx-remote>", 0)
	p.send("printf '%s\\n' \"$$\" > /home/tester/transport.pid\r")
	s.wait("shell PID", func() bool {
		_, err := s.command(5*time.Second, f.ssh, f.target, "test -s /home/tester/transport.pid")
		return err == nil
	})
	pid := strings.TrimSpace(f.cli("cat /home/tester/transport.pid"))
	f.proxy.setOffline(true)
	select {
	case <-p.exitDone:
	case <-time.After(20 * time.Second):
		t.Fatal("remote client did not return after connection loss")
	}
	p.close()
	s.cli("docker", "exec", f.container, "kill", "-0", pid)
	f.proxy.setOffline(false)
	if !f.running(name) {
		t.Fatal("network loss stopped remote session")
	}
	p = s.terminal("transport-reconnect", f.herdr, "--remote", f.target, "--session", name)
	p.expect("hctx-remote>", 0)
	f.cli("rm /home/tester/transport.pid")
	p.send("printf '%s\\n' \"$$\" > /home/tester/transport.pid\r")
	s.wait("reattached shell PID", func() bool {
		_, err := s.command(5*time.Second, f.ssh, f.target, "test -s /home/tester/transport.pid")
		return err == nil
	})
	if got := strings.TrimSpace(f.cli("cat /home/tester/transport.pid")); got != pid {
		t.Fatalf("shell replaced after transport loss: %s -> %s", pid, got)
	}
	p.send("\x02q")
	p.waitExit()
	s.check("transport failure preserves remote shell and permits reattach")
}

func TestRemoteAuthenticationAndPathFailures(t *testing.T) {
	if !*remoteFlag {
		t.Skip("enable with -integration.remote")
	}
	f := newRemoteFixture(t, "0.8.2")
	s := f.s
	config := s.env["FIXTURE_SSH_CONFIG"]
	configRaw := readFile(t, config)
	secrets := filepath.Dir(config)
	known := filepath.Join(secrets, "known_hosts")
	knownRaw := readFile(t, known)
	fields := strings.Fields(string(knownRaw))
	wrongPublic := readFile(t, filepath.Join(secrets, "id_ed25519.pub"))
	writeFile(t, known, []byte(fields[0]+" "+string(wrongPublic)), 0o600)
	output, err := s.command(5*time.Second, f.ssh, f.target, "true")
	if err == nil || !strings.Contains(output, "HOST IDENTIFICATION HAS CHANGED") {
		t.Fatalf("host key failure: %v %s", err, output)
	}
	writeFile(t, known, knownRaw, 0o600)
	wrongIdentity := strings.ReplaceAll(string(configRaw), filepath.Join(secrets, "id_ed25519"), filepath.Join(secrets, "host_ed25519"))
	writeFile(t, config, []byte(wrongIdentity), 0o600)
	output, err = s.command(5*time.Second, f.ssh, f.target, "true")
	if err == nil || !strings.Contains(output, "Permission denied") {
		t.Fatalf("identity failure: %v %s", err, output)
	}
	writeFile(t, config, configRaw, 0o600)
	f.cli("true")
	s.cli("docker", "exec", f.container, "mv", "/usr/local/bin/herdr", "/usr/local/bin/herdr-unavailable")
	p := s.terminal("missing-remote-herdr", repoPath(*binaryFlag), "--herdr-bin", f.herdr, "--remote", f.target, "--interval", "500ms")
	p.expect("Refresh failed", 0)
	p.expect("remote Herdr on PATH", 0)
	p.quit()
	s.check("wrong host key, unauthorized identity, and missing remote binary fail visibly")
}
