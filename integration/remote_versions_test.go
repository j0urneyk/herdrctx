//go:build integration

package integration

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j0urneyk/herdrctx/internal/preferences"
)

func TestRemoteMixedVersions(t *testing.T) {
	if !*remoteFlag {
		t.Skip("enable with -integration.remote")
	}
	for _, pair := range [][2]string{{"0.8.2", "0.9.0"}, {"0.9.0", "0.8.2"}} {
		for _, mode := range []string{"management", "release-decline", "decline", "accept", "running", "failure", "cancel"} {
			t.Run(pair[0]+"-client-"+pair[1]+"-server/"+mode, func(t *testing.T) {
				f := newRemoteFixture(t, pair[1])
				sentinel := f.seedVersionSession("sentinel", f.herdr)
				name := "mixed"
				previous := ""
				if mode == "running" || mode == "management" {
					previous = f.seedVersionSession(name, f.herdr)
				}
				f.useVersionClient(pair[0])
				if mode == "release-decline" {
					delete(f.s.env, "HERDR_REMOTE_BINARY")
				}
				original := f.versionFingerprint()
				f.s.writeJSON("before.json", map[string]any{"installation": original, "sessions": f.sessions(), "sentinel_pid": sentinel, "selected_pid": previous})
				if mode == "failure" || mode == "cancel" {
					f.cli("mkdir -p /home/tester/.local/bin; cp /usr/local/bin/herdr /home/tester/.local/bin/herdr")
					f.installTransferFault(mode)
				}
				p := f.s.terminal("mixed-app", repoPath(*binaryFlag), "--herdr-bin", f.herdr, "--remote", f.target, "--interval", "500ms")
				p.expect(fmt.Sprintf("Loaded %d session(s).", len(f.sessions())), 0)
				if mode == "management" {
					p.send("/" + name + enter)
					f.stopDeleteVersionSession(p, name, previous)
					f.assertVersionSession("sentinel", sentinel)
					f.assertVersionUnchanged(original)
					f.cli("test ! -e /home/tester/.local/bin/herdr")
					p.quit()
					f.s.check("mixed local CLI and original remote PATH list/stop/delete without native installation")
					return
				}
				if previous != "" {
					p.send("/" + name + enter)
					p.send(enter)
					p.expect(runningVersionInstallPrompt(pair[0]), 0)
					offset := p.offset()
					p.send("n" + enter)
					p.expect("Attach failed", offset)
					f.assertVersionSession(name, previous)
					f.assertVersionUnchanged(original)
					p.send(enter)
					p.send(enter)
					p.expect(runningVersionInstallPrompt(pair[0]), offset)
				} else {
					p.sendExpect("n", "Uses the remote default directory.")
					p.send(name + enter)
					if mode == "release-decline" {
						p.expect("Install the "+pair[0]+" stable asset", 0)
					} else {
						p.expect("Install HERDR_REMOTE_BINARY", 0)
					}
				}
				switch mode {
				case "decline", "release-decline":
					p.sendExpect("n"+enter, "Attach failed")
					f.assertVersionUnchanged(original)
					f.cli("test ! -e /home/tester/.local/bin/herdr")
					f.assertNoVersionSession(name)
					p.send(enter)
				case "failure", "cancel":
					p.send("y" + enter)
					f.s.wait("install transfer reached fault gate", func() bool {
						_, err := f.s.command(3*time.Second, f.ssh, f.target, "test -s /home/tester/install-transfer")
						return err == nil
					})
					if mode == "cancel" {
						p.send("\x03")
					}
					p.expect("Attach failed", 0)
					if mode == "cancel" {
						p.expect("signal: interrupt", 0)
					}
					p.send(enter)
					f.assertVersionUnchanged(original)
					f.checkInstalledAsset(pair[1])
					f.assertNoVersionSession(name)
					leftovers := f.cli("find /home/tester/.local/bin -maxdepth 1 -type f -printf '%f %s bytes\\n'; ps -u tester -o pid,args")
					writeFile(t, filepath.Join(f.s.artifacts, "interrupted-install.txt"), []byte(leftovers), 0o600)
					f.s.check("interrupted install preserves existing destination binary, original PATH/config/session; residual files and processes recorded before container cleanup")
				default:
					p.send("y" + enter)
					if previous != "" && pair[0] == "0.8.2" {
						p.expect("remote herdr server must restart before this bridge can attach", 0)
						p.expect("Attach failed", 0)
						f.assertVersionSession(name, previous)
						f.checkInstalledAsset(pair[0])
						f.recordVersionServer(pair[1], previous, "/usr/local/bin/herdr")
						p.send(enter)
						f.stopDeleteVersionSession(p, name, previous)
						f.s.check("0.8.2 refuses a running 0.9.0 server after installation; original shell survives until explicit stop")
						break
					}
					p.expect("hctx-remote>", 0)
					pid := f.observeVersionShell(p, name)
					if previous != "" {
						if pid == previous {
							t.Fatal("explicit replacement did not replace old shell")
						}
						f.waitShellExit(previous)
					}
					f.assertVersionUnchanged(original)
					f.recordInstalledVersion(pair[0], pid)
					f.s.check("installed version ready for cancellation")
					if *failAfterAttachFlag {
						t.Fatal("Injected failure after mixed-version installation")
					}
					offset := p.offset()
					p.send("\x02q")
					p.expect("Loaded 3 session(s).", offset)
					p.quit()
					// The explicit binary override requests installation on every launch.
					// Remove it before testing ordinary discovery and reattachment.
					delete(f.s.env, "HERDR_REMOTE_BINARY")
					p = f.s.terminal("reattach-app", repoPath(*binaryFlag), "--herdr-bin", f.herdr, "--remote", f.target, "--interval", "500ms")
					p.expect("Loaded 3 session(s).", 0)
					p.send("/" + name + enter)
					offset = p.offset()
					p.send(enter)
					p.expect("HCTX_VERSION_MARKER", offset)
					if got := f.observeVersionShell(p, name); got != pid {
						t.Fatalf("reattach replaced shell %s with %s", pid, got)
					}
					offset = p.offset()
					p.send("\x02q")
					p.expect("Loaded 3 session(s).", offset)
					f.stopDeleteVersionSession(p, name, pid)
				}
				f.assertVersionSession("sentinel", sentinel)
				f.assertVersionUnchanged(original)
				p.quit()
				f.s.check("mixed version management and foreground result verified; unrelated original-version session preserved")
			})
		}
	}
}

func (f *remoteFixture) useVersionClient(version string) {
	s := f.s
	f.herdr = s.herdr(version, "")
	target := s.metadata["remote_platform"].(string)
	s.env["HERDR_REMOTE_BINARY"] = s.herdrAsset(version, target)
	s.metadata["client_version"] = version
	s.metadata["install_asset_sha256"] = testDigests[version][target]
	s.metadata["initial_path_version"] = strings.TrimSpace(f.cli("herdr --version"))
	s.metadata["git_head"] = strings.TrimSpace(s.cli("git", "-C", repo, "rev-parse", "HEAD"))
	s.metadata["git_status"] = s.cli("git", "-C", repo, "status", "--short")
}

func (f *remoteFixture) versionFingerprint() string {
	return f.cli("sha256sum /usr/local/bin/herdr /home/tester/.config/herdr/config.toml; command -v herdr; herdr --version; id -un; printf '%s\\n' \"$HOME\"")
}

func (f *remoteFixture) assertVersionUnchanged(before string) {
	f.s.t.Helper()
	if after := f.versionFingerprint(); after != before {
		f.s.t.Fatalf("original installation or config changed:\nbefore %s\nafter %s", before, after)
	}
}

func (f *remoteFixture) seedVersionSession(name, binary string) string {
	p := f.s.terminal("seed-"+name, binary, "--remote", f.target, "--session", name)
	p.expect("hctx-remote>", 0)
	pid := f.observeVersionShell(p, name)
	p.send("\x02q")
	p.waitExit()
	return pid
}

func (f *remoteFixture) observeVersionShell(p *terminal, name string) string {
	f.s.t.Helper()
	file := "/home/tester/" + name + ".pid"
	f.cli("rm -f " + quote(file))
	p.send("printf '%s\\n' \"$$\" > " + quote(file) + "; printf '%s%s\\n' HCTX_VERSION_ MARKER\r")
	f.s.wait("version shell command executed", func() bool {
		_, err := f.s.command(3*time.Second, f.ssh, f.target, "test -s "+quote(file))
		return err == nil
	})
	pid := strings.TrimSpace(f.cli("cat " + quote(file)))
	f.assertVersionSession(name, pid)
	return pid
}

func (f *remoteFixture) assertVersionSession(name, pid string) {
	f.s.t.Helper()
	if !f.running(name) {
		f.s.t.Fatalf("session %s no longer running", name)
	}
	state := strings.TrimSpace(f.cli("ps -o stat= -p " + quote(pid) + " || true"))
	if state == "" || strings.HasPrefix(state, "Z") {
		f.s.t.Fatalf("original shell %s is no longer alive", pid)
	}
}

func (f *remoteFixture) assertNoVersionSession(name string) {
	f.s.t.Helper()
	for _, row := range f.sessions() {
		if row.Name == name {
			f.s.t.Fatalf("unexpected session %s", name)
		}
	}
}

func (f *remoteFixture) checkInstalledAsset(version string) {
	f.s.t.Helper()
	path := "/home/tester/.local/bin/herdr"
	got := strings.TrimSpace(f.cli(quote(path) + " --version"))
	expected := testDigests[version][f.s.metadata["remote_platform"].(string)]
	checksum := strings.Fields(f.cli("sha256sum " + quote(path)))[0]
	if got != "herdr "+version || checksum != expected {
		f.s.t.Fatalf("installed asset: version %s, hash %s", got, checksum)
	}
	f.s.metadata["installed_version"] = got
	f.s.metadata["installed_sha256"] = checksum
}

func (f *remoteFixture) recordInstalledVersion(version, pid string) {
	f.checkInstalledAsset(version)
	f.recordVersionServer(version, pid, "/home/tester/.local/bin/herdr")
}

func (f *remoteFixture) recordVersionServer(version, pid, path string) {
	f.s.t.Helper()
	// The shell parent identifies the running server independently of PATH.
	serverPID := strings.TrimSpace(f.cli("ps -o ppid= -p " + quote(pid)))
	proc := "/proc/" + serverPID + "/exe"
	executable := strings.TrimSpace(f.cli("readlink " + quote(proc)))
	if executable != path {
		f.s.t.Fatalf("server executable: %s", executable)
	}
	checksum := strings.Fields(f.cli("sha256sum " + quote(proc)))[0]
	if checksum != testDigests[version][f.s.metadata["remote_platform"].(string)] {
		f.s.t.Fatal("running server hash differs from expected release")
	}
	f.s.metadata["native_server_version"] = version
	f.s.metadata["native_server_executable"] = executable
	f.s.metadata["native_server_pid"] = serverPID
	f.s.writeJSON("after-install.json", map[string]any{"management": f.versionFingerprint(), "sessions": f.sessions(), "native_server": executable, "native_sha256": checksum})
}

func (f *remoteFixture) stopDeleteVersionSession(p *terminal, name, pid string) {
	p.sendExpect("s", fmt.Sprintf("Stop session %q on %s?", name, f.target))
	offset := p.offset()
	p.send(enter)
	f.s.wait("version session stopped", func() bool { return !f.running(name) })
	f.waitShellExit(pid)
	p.expect("stopped", offset)
	p.sendExpect("d", fmt.Sprintf("Delete session %q on %s?", name, f.target))
	p.send(enter)
	f.s.wait("version session deleted", func() bool {
		for _, row := range f.sessions() {
			if row.Name == name {
				return false
			}
		}
		return true
	})
}

func (f *remoteFixture) installTransferFault(mode string) {
	// Only Herdr's upload tee is intercepted; all discovery and management SSH stay real.
	script := "#!/bin/sh\ncase \"$1\" in /home/tester/.local/bin/herdr.tmp.*) ;; *) exec /usr/bin/tee \"$@\" ;; esac\n/usr/bin/dd bs=4096 count=1 of=\"$1\" 2>/dev/null\nprintf '%s\\n' \"$$\" > /home/tester/install-transfer\n"
	if mode == "failure" {
		script += "exit 42\n"
	} else {
		script += "exec sleep 60\n"
	}
	f.s.cli("docker", "exec", "--user", "root", f.container, "sh", "-c", "printf %s "+quote(script)+" > /usr/local/bin/tee; chmod 755 /usr/local/bin/tee")
}

func TestRemoteMachineVersions(t *testing.T) {
	if !*remoteFlag {
		t.Skip("enable with -integration.remote")
	}
	for _, pair := range [][2]string{{"0.8.2", "0.9.0"}, {"0.9.0", "0.8.2"}} {
		t.Run(pair[0]+"-client-"+pair[1]+"-server", func(t *testing.T) {
			f := newRemoteFixture(t, pair[1])
			f.useVersionClient(pair[0])
			before := ""
			sourceHash := ""
			sourcePath := filepath.Join(f.s.env["XDG_STATE_HOME"], "herdr", "client", "endpoints.json")
			if pair[0] == "0.9.0" {
				p := f.s.terminal("machine-setup", f.herdr, "machine", "add", f.target, "--label", "Mixed", "--remote-session", "profile")
				p.expect("Install HERDR_REMOTE_BINARY", 0)
				p.send("y" + enter)
				p.waitExit()
				before = f.s.cli(f.herdr, "machine", "list", "--json")
				sourceHash = digest(readFile(t, sourcePath))
				f.s.metadata["machine_catalog_sha256"] = sourceHash
			}
			path := filepath.Join(f.s.root, "hosts.json")
			p := f.s.terminal("machine-import", repoPath(*binaryFlag), "--herdr-bin", f.herdr, "--remote", f.target, "--hosts-file", path, "--interval", "500ms")
			p.expect(fmt.Sprintf("Loaded %d session(s).", len(f.sessions())), 0)
			p.sendExpect("H", "Hosts")
			p.send("i")
			if pair[0] == "0.8.2" {
				p.expect("machine import requires local Herdr 0.9.0 or newer", 0)
			} else {
				p.expect("Import a machine as a saved host", 0)
				p.send(enter)
				f.s.wait("version profile imported", func() bool {
					d, err := (&preferences.HostStore{Path: path}).Load()
					return err == nil && len(d.Items) == 1 && d.Items[0].Destination == f.target && d.Items[0].Session == "profile"
				})
				if after := f.s.cli(f.herdr, "machine", "list", "--json"); after != before || digest(readFile(t, sourcePath)) != sourceHash {
					t.Fatal("import changed source catalog")
				}
			}
			p.sendExpect(esc, "enter/a")
			p.quit()
			f.s.check("machine import minimum is the local CLI version, independent of remote PATH version")
		})
	}
}

func runningVersionInstallPrompt(client string) string {
	if client == "0.9.0" {
		return "Install 0.9.0 and stop the remote server now? [y/N]"
	}
	return "Install HERDR_REMOTE_BINARY"
}
