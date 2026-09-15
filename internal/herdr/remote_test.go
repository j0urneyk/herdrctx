package herdr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRemoteTargetValidation(t *testing.T) {
	for _, value := range []string{"workbox", "user@server", "ssh://user@server:2222", "ssh://user@[::1]:2222"} {
		target, err := ParseRemoteTarget(value)
		if err != nil || target.Destination != value {
			t.Fatalf("%q: %v %v", value, target, err)
		}
	}
	for _, value := range []string{"", "-oProxyCommand=x", "user@", "@host", "a@@host", "host:2222", "host;touch x", "a b", "a\nb", "$(id)", "ssh://user:password@host", "ssh://host/path", "ssh://host:65536", "ssh://host:abc", "ssh://"} {
		if _, err := ParseRemoteTarget(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}

func remoteClientFixture(t *testing.T, body string) (*Client, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	script := []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$TEST_SSH_ARGS\"\n" + body)
	// #nosec G306 -- The isolated fake SSH must be executable by this test's user.
	if err := os.WriteFile(filepath.Join(dir, "ssh"), script, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_SSH_ARGS", log)
	c := NewClient("herdr")
	c.Remote = &RemoteTarget{Destination: "user@workbox"}
	return c, log
}

func TestRemoteListAndCommandBoundary(t *testing.T) {
	c, log := remoteClientFixture(t, `printf 'herdr 0.8.2\n{"sessions":[{"name":"api","running":true}]}\n'`)
	entries, err := c.ListSessions(context.Background())
	if err != nil || len(entries) != 1 || entries[0].ID() != (SessionID{Target: "user@workbox", Name: "api"}) {
		t.Fatalf("%+v %v", entries, err)
	}
	// #nosec G304 -- Read the argument log created by this test's fixture.
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2", "--", "user@workbox", "herdr --version && herdr session list --json"}
	if !reflect.DeepEqual(strings.Split(strings.TrimSpace(string(raw)), "\n"), want) {
		t.Fatalf("arguments: %s", raw)
	}
	cmd, err := c.CreateAttachCommand("api", "/does/not/exist")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Dir != "" || !reflect.DeepEqual(cmd.Args, []string{"herdr", "--remote", "user@workbox", "--session", "api"}) {
		t.Fatalf("command %+v", cmd)
	}
}

func TestRemoteListRejectsUnsupportedOrMalformedOutput(t *testing.T) {
	for _, body := range []string{`printf 'herdr 0.6.5\n{"sessions":[]}'`, `printf 'banner\n{"sessions":[]}'`, `printf 'herdr 0.9.0\nnot-json'`} {
		t.Run(body, func(t *testing.T) {
			c, _ := remoteClientFixture(t, body)
			if _, err := c.ListSessions(context.Background()); err == nil {
				t.Fatal("accepted invalid list")
			}
		})
	}
}

func TestRemoteMutationUnknownIsNotRetried(t *testing.T) {
	c, log := remoteClientFixture(t, "echo connection-lost >&2; exit 255")
	err := c.StopSession(context.Background(), "api")
	var unknown *OutcomeUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected unknown, got %v", err)
	}
	// #nosec G304 -- Read the argument log created by this test's fixture.
	raw, _ := os.ReadFile(log)
	if strings.Count(string(raw), "herdr session stop 'api' --json") != 1 {
		t.Fatalf("replayed command: %s", raw)
	}
	if !strings.Contains(err.Error(), "user@workbox") {
		t.Fatal(err)
	}
}

func TestRemoteStructuredErrorRemainsDefinitive(t *testing.T) {
	c, _ := remoteClientFixture(t, `echo '{"error":{"code":"session_running","message":"stop first"}}' >&2; exit 1`)
	err := c.DeleteSession(context.Background(), "api")
	var unknown *OutcomeUnknownError
	var ce *CommandError
	if errors.As(err, &unknown) || !errors.As(err, &ce) || ce.HerdrCode != "session_running" {
		t.Fatal(err)
	}
}

func TestRemoteTimeoutCancelsLocalSSH(t *testing.T) {
	c, _ := remoteClientFixture(t, "sleep 10")
	c.Timeout = 40 * time.Millisecond
	start := time.Now()
	err := c.StopSession(context.Background(), "api")
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatalf("timeout: %v", err)
	}
	var unknown *OutcomeUnknownError
	if !errors.As(err, &unknown) {
		t.Fatal("timeout claimed remote cancellation")
	}
}
