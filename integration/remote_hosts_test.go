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
	for _, versions := range [][2]string{{"0.8.2", "0.9.0"}, {"0.9.0", "0.8.2"}} {
		t.Run(versions[0]+"-client-"+versions[1]+"-server", func(t *testing.T) {
			f := newRemoteFixture(t, versions[1])
			f.herdr = f.s.herdr(versions[0], "")
			p := f.s.terminal("mixed", f.herdr, "--remote", f.target, "--session", "mixed")
			p.expect("Install the "+versions[0]+" stable asset", 0)
			p.send("n" + enter)
			select {
			case <-p.exitDone:
			case <-time.After(5 * time.Second):
				t.Fatal("refused install did not return")
			}
			p.close()
			if got := strings.TrimSpace(f.cli("herdr --version")); got != "herdr "+versions[1] {
				t.Fatalf("remote installation changed: %s", got)
			}
			f.cli("test ! -e /home/tester/.local/bin/herdr")
			f.s.check("mixed versions require matching remote binary; installation declined and original installation preserved")
		})
	}
}

func TestRemoteMultipleHosts(t *testing.T) {
	if !*remoteFlag {
		t.Skip("enable with -integration.remote")
	}
	one := newRemoteFixture(t, "0.9.0")
	two := newRemoteFixture(t, "0.9.0")
	s := one.s
	config := strings.Replace(string(readFile(t, s.env["FIXTURE_SSH_CONFIG"])), "Host *", "Host one", 1) + "\n" + strings.Replace(string(readFile(t, two.s.env["FIXTURE_SSH_CONFIG"])), "Host *", "Host two", 1)
	writeFile(t, s.env["FIXTURE_SSH_CONFIG"], []byte(config), 0o600)
	one.target = "one"
	for _, id := range []string{"one", "two"} {
		p := s.terminal("seed-"+id, one.herdr, "--remote", id, "--session", "api")
		p.expect("hctx-remote>", 0)
		p.send("\x02q")
		p.waitExit()
	}
	store := &preferences.HostStore{Path: filepath.Join(s.root, "hosts.json")}
	for _, id := range []string{"one", "two"} {
		_, err := store.Apply(preferences.HostChange{Host: preferences.Host{Label: id, Destination: id}})
		must(t, err)
	}
	p := s.terminal("multiple-hosts", repoPath(*binaryFlag), "--herdr-bin", one.herdr, "--all-hosts", "--hosts-file", store.Path, "--interval", "500ms")
	p.expect("two: 2 sessions", 0)
	s.check("both hosts ready for cancellation")
	if *failAfterAttachFlag {
		t.Fatal("Injected failure with two remote hosts")
	}
	p.sendExpect("H", "Hosts")
	p.send("\x1b[B")
	p.send(enter)
	p.send("/api" + enter)
	p.sendExpect("i", "Host: one")
	p.sendExpect(enter, "enter/a")
	p.sendExpect(enter, "hctx-remote>")
	p.send("\x02q")
	p.expect("enter/a", p.after("hctx-remote>"))
	p.sendExpect("H", "Hosts")
	p.send("\x1b[B\x1b[B")
	p.send(enter)
	p.sendExpect("s", fmt.Sprintf("Stop session %q on two?", "api"))
	p.send(enter)
	s.wait("only second host stopped", func() bool { return !two.running("api") })
	if !one.running("api") {
		t.Fatal("stop routed to wrong host")
	}
	p.sendExpect("H", "Hosts")
	p.send("A")
	two.proxy.setOffline(true)
	p.expect("Refresh failed on two", 0)
	if !one.running("api") {
		t.Fatal("other host unavailable")
	}
	two.proxy.setOffline(false)
	p.send("r")
	s.wait("second SSH recovered", func() bool { _, err := two.s.command(3*time.Second, two.ssh, two.target, "true"); return err == nil })
	p.quit()
	// Create a real profile and exercise the app's importer against the public CLI.
	s.cli(one.herdr, "machine", "add", "one", "--label", "Imported", "--remote-session", "api")
	p = s.terminal("real-machine-import", repoPath(*binaryFlag), "--herdr-bin", one.herdr, "--all-hosts", "--hosts-file", store.Path, "--interval", "500ms")
	p.expect("one: 2 sessions", 0)
	p.sendExpect("H", "Hosts")
	p.sendExpect("i", "Import a machine as a saved host")
	p.sendExpect(enter, "Replace one")
	p.send("y")
	s.wait("real profile imported", func() bool {
		d, err := store.Load()
		return err == nil && d.Items[0].ProfileID != "" && d.Items[0].Session == "api"
	})
	p.sendExpect(esc, "enter/a")
	p.quit()
	s.check("two independent hosts, same-name selection and handoff, correct stop, partial outage, real machine import")
}

func TestRemoteDelayedTransport(t *testing.T) {
	if !*remoteFlag {
		t.Skip("enable with -integration.remote")
	}
	f := newRemoteFixture(t, "0.8.2")
	f.proxy.setTransport(5*time.Millisecond, false)
	if len(f.sessions()) == 0 {
		t.Fatal("delayed list empty")
	}
	f.proxy.setTransport(0, true)
	_, err := f.s.command(250*time.Millisecond, f.ssh, f.target, "herdr session list --json")
	if err == nil {
		t.Fatal("stalled transport did not time out")
	}
	f.proxy.setTransport(0, false)
	f.sessions()
	f.s.check("delayed query succeeds, stalled transport times out, query recovers without restarting server")
}

func TestRemoteJumpHost(t *testing.T) {
	if !*remoteFlag {
		t.Skip("enable with -integration.remote")
	}
	target := newRemoteFixture(t, "0.9.0")
	jump := newRemoteFixture(t, "0.9.0")
	s := target.s
	address := strings.TrimSpace(s.cli("docker", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", target.container))
	if address == "" {
		t.Fatal("target bridge address missing")
	}
	base := string(readFile(t, s.env["FIXTURE_SSH_CONFIG"]))
	targetConfig := strings.Replace(base, "Host *", "Host via-jump", 1)
	targetConfig = strings.Replace(targetConfig, "HostName 127.0.0.1", "HostName "+address, 1)
	lines := strings.Split(targetConfig, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "  Port ") {
			lines[i] = "  Port 22"
		}
	}
	targetConfig = strings.Join(lines, "\n") + "  ProxyJump bastion\n  HostKeyAlias hctx-target\n"
	known := filepath.Join(filepath.Dir(s.env["FIXTURE_SSH_CONFIG"]), "known_hosts")
	writeFile(t, known, append([]byte("hctx-target "), readFile(t, filepath.Join(filepath.Dir(known), "host_ed25519.pub"))...), 0o600)
	jumpConfig := strings.Replace(string(readFile(t, jump.s.env["FIXTURE_SSH_CONFIG"])), "Host *", "Host bastion", 1)
	writeFile(t, s.env["FIXTURE_SSH_CONFIG"], []byte(targetConfig+"\n"+jumpConfig), 0o600)
	target.target = "via-jump"
	target.sessions()
	p := s.terminal("jump-native", target.herdr, "--remote", target.target, "--session", "jump-api")
	p.expect("hctx-remote>", 0)
	p.send("\x02q")
	p.waitExit()
	if !target.running("jump-api") {
		t.Fatal("jump connection did not reach target")
	}
	target.cli("herdr session stop jump-api --json")
	target.cli("herdr session delete jump-api --json")
	s.check("management and native SSH use isolated ProxyJump, strict host keys and separate target/bastion identities")
}
