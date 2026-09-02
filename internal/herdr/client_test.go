package herdr

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestClientListSessions(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeHerdr(t, `
if [ "$1" = "session" ] && [ "$2" = "list" ] && [ "$3" = "--json" ]; then
  printf '{"sessions":[{"default":false,"name":"work","running":false,"session_dir":"/tmp/work","socket_path":"/tmp/work/herdr.sock"}]}'
  exit 0
fi
printf 'unexpected args: %s %s %s\n' "$1" "$2" "$3" >&2
exit 2
`))

	sessions, err := client.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].Name != "work" || sessions[0].Running {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
}

func TestClientEnsureMinimumVersionAcceptsRequiredVersion(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeHerdr(t, `
if [ "$1" = "--version" ]; then
  printf 'herdr 0.6.5\n'
  exit 0
fi
printf 'unexpected args: %s\n' "$1" >&2
exit 2
`))

	if err := client.EnsureMinimumVersion(context.Background()); err != nil {
		t.Fatalf("EnsureMinimumVersion() error = %v", err)
	}
}

func TestClientEnsureMinimumVersionRejectsOlderVersion(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeHerdr(t, `
if [ "$1" = "--version" ]; then
  printf 'herdr 0.6.4\n'
  exit 0
fi
printf 'unexpected args: %s\n' "$1" >&2
exit 2
`))

	err := client.EnsureMinimumVersion(context.Background())
	if err == nil {
		t.Fatal("EnsureMinimumVersion() error = nil, want version error")
	}
	if !strings.Contains(err.Error(), "requires herdr 0.6.5 or newer") {
		t.Fatalf("EnsureMinimumVersion() error = %q, want requirement", err.Error())
	}
	if !strings.Contains(err.Error(), "found 0.6.4") {
		t.Fatalf("EnsureMinimumVersion() error = %q, want found version", err.Error())
	}
}

func TestParseVersionOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   semanticVersion
	}{
		{name: "plain", output: "0.6.5\n", want: semanticVersion{major: 0, minor: 6, patch: 5, raw: "0.6.5"}},
		{name: "with name", output: "herdr v0.10.1\n", want: semanticVersion{major: 0, minor: 10, patch: 1, raw: "0.10.1"}},
		{name: "with suffix", output: "herdr 1.2.3+abc\n", want: semanticVersion{major: 1, minor: 2, patch: 3, raw: "1.2.3+abc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseVersionOutput([]byte(tt.output))
			if err != nil {
				t.Fatalf("parseVersionOutput() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseVersionOutput() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseVersionOutputRejectsUnknownFormat(t *testing.T) {
	t.Parallel()

	_, err := parseVersionOutput([]byte("herdr dev\n"))
	if err == nil {
		t.Fatal("parseVersionOutput() error = nil, want parse error")
	}
}

func TestClientListSessionsAcceptsLargeOutput(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeHerdr(t, `
if [ "$1" = "session" ] && [ "$2" = "list" ] && [ "$3" = "--json" ]; then
  printf '{"sessions":['
  i=0
  while [ "$i" -lt 700 ]; do
    if [ "$i" -gt 0 ]; then
      printf ','
    fi
    printf '{"default":false,"name":"work%s","running":false,"session_dir":"/tmp/%0128d","socket_path":"/tmp/%0128d/herdr.sock"}' "$i" "$i" "$i"
    i=$((i + 1))
  done
  printf ']}'
  exit 0
fi
printf 'unexpected args: %s %s %s\n' "$1" "$2" "$3" >&2
exit 2
`))

	sessions, err := client.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 700 {
		t.Fatalf("len(sessions) = %d, want 700", len(sessions))
	}
}

func TestClientCreateAttachCommand(t *testing.T) {
	t.Parallel()

	client := NewClient("/usr/local/bin/herdr")
	dir := t.TempDir()
	for _, name := range []string{"work", "A", "0", "work-1_2.3", strings.Repeat("a", 64)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd, err := client.CreateAttachCommand(name, dir)
			if err != nil {
				t.Fatalf("CreateAttachCommand() error = %v", err)
			}

			if cmd.Path != "/usr/local/bin/herdr" {
				t.Fatalf("Path = %q", cmd.Path)
			}
			if got := strings.Join(cmd.Args, " "); got != "/usr/local/bin/herdr --session "+name {
				t.Fatalf("Args = %q", got)
			}
			if cmd.Dir != dir {
				t.Fatalf("Dir = %q", cmd.Dir)
			}
		})
	}
}

func TestClientCreateAttachCommandRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	client := NewClient("/usr/local/bin/herdr")
	dir := t.TempDir()
	for _, name := range []string{
		"", "bad name", "work/1", "work;1", "한글", strings.Repeat("a", 65),
		"-", "--", "-work", "--json", "--session", "--help", "-h", "_work", ".work", "help",
	} {
		if cmd, err := client.CreateAttachCommand(name, dir); err == nil || cmd != nil {
			t.Fatalf("CreateAttachCommand(%q) = %v, %v; want nil command and validation error", name, cmd, err)
		}
	}
	if _, err := client.CreateAttachCommand("work", ""); err == nil {
		t.Fatal("CreateAttachCommand() error = nil, want start directory error")
	}
}

func TestClientAttachCommandRejectsHelpNames(t *testing.T) {
	t.Parallel()

	client := NewClient("/usr/local/bin/herdr")
	for _, name := range []string{"help", "--help"} {
		if _, err := client.AttachCommand(name); err == nil {
			t.Fatalf("AttachCommand(%q) error = nil, want attachability error", name)
		}
	}

	cmd, err := client.AttachCommand("-work")
	if err != nil {
		t.Fatalf("AttachCommand(-work) error = %v", err)
	}
	if got := strings.Join(cmd.Args, " "); got != "/usr/local/bin/herdr session attach -work" {
		t.Fatalf("Args = %q", got)
	}
}

func TestClientActionsRejectInvalidSessionNames(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeHerdr(t, `
exit 0
`))

	if err := client.StopSession(context.Background(), "bad name"); err == nil {
		t.Fatal("StopSession() error = nil, want invalid session name error")
	}
	if err := client.DeleteSession(context.Background(), " work"); err == nil {
		t.Fatal("DeleteSession() error = nil, want invalid session name error")
	}
}

func TestClientActionsRejectJSONFlagSessionName(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeHerdr(t, `
exit 0
`))

	if err := client.StopSession(context.Background(), "--json"); err == nil {
		t.Fatal("StopSession() error = nil, want reserved session name error")
	}
	if err := client.DeleteSession(context.Background(), "--json"); err == nil {
		t.Fatal("DeleteSession() error = nil, want reserved session name error")
	}
}

func TestCommandErrorUsesShortOutputPreview(t *testing.T) {
	t.Parallel()

	err := (&CommandError{Args: []string{"herdr", "session", "list"}, Stdout: strings.Repeat("x", maxCommandErrorPreviewBytes+100)}).Error()
	if strings.Count(err, "x") > maxCommandErrorPreviewBytes {
		t.Fatalf("error preview contains too much stdout: len=%d", strings.Count(err, "x"))
	}
	if !strings.Contains(err, "output preview truncated") {
		t.Fatalf("error = %q, want truncation marker", err)
	}
}

func TestClientCommandOutputIsCapped(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeHerdr(t, `
dd if=/dev/zero bs=1024 count=80 2>/dev/null | tr '\000' x
exit 1
`))

	err := client.DeleteSession(context.Background(), "work")
	if err == nil {
		t.Fatal("DeleteSession() error = nil, want command error")
	}

	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("DeleteSession() error type = %T, want *CommandError", err)
	}
	if len(commandErr.Stdout) > maxCommandOutputBytes+128 {
		t.Fatalf("captured stdout length = %d, want capped near %d", len(commandErr.Stdout), maxCommandOutputBytes)
	}
	if !strings.Contains(commandErr.Stdout, "output truncated after") {
		t.Fatalf("stdout = %q, want truncation marker", commandErr.Stdout)
	}
}

func TestClientCommandWaitDelayBoundsInheritedPipes(t *testing.T) {
	t.Parallel()

	marker := filepath.Join(t.TempDir(), "survived")
	client := NewClient(fakeHerdr(t, `
if [ "$1" = "session" ] && [ "$2" = "list" ] && [ "$3" = "--json" ]; then
  (sleep 1; printf survived > `+shellQuote(marker)+`) &
  printf '{"sessions":[]}'
  exit 0
fi
exit 2
`))
	client.WaitDelay = 20 * time.Millisecond

	_, err := client.ListSessions(context.Background())
	if err == nil {
		t.Fatal("ListSessions() error = nil, want wait delay error")
	}
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("ListSessions() error = %v, want exec.ErrWaitDelay", err)
	}
	if !strings.Contains(err.Error(), "I/O wait timed out") {
		t.Fatalf("ListSessions() error = %q, want wait delay message", err.Error())
	}

	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("descendant marker exists, want process group cleanup")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat descendant marker: %v", err)
	}
}

func TestClientCommandErrorIncludesHerdrError(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeHerdr(t, `
printf '{"error":{"code":"session_delete_failed","message":"deleting the default session is not supported"}}' >&2
exit 1
`))

	err := client.DeleteSession(context.Background(), "default")
	if err == nil {
		t.Fatal("DeleteSession() error = nil, want error")
	}

	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("DeleteSession() error type = %T, want *CommandError", err)
	}
	if commandErr.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", commandErr.ExitCode)
	}
	if commandErr.HerdrCode != "session_delete_failed" {
		t.Fatalf("HerdrCode = %q", commandErr.HerdrCode)
	}
	if !strings.Contains(err.Error(), "deleting the default session") {
		t.Fatalf("error message = %q", err.Error())
	}
}

func fakeHerdr(t *testing.T, body string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("shell fake is Unix-only")
	}

	path := filepath.Join(t.TempDir(), "herdr")
	content := "#!/bin/sh\n" + body + "\n"
	// #nosec G306 -- The fake Herdr command must be executable for the test.
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake herdr: %v", err)
	}

	return path
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
