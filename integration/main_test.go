//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	binaryFlag          = flag.String("integration.binary", "bin/herdrctx", "herdrctx binary, relative to the repository or absolute")
	herdrFlag           = flag.String("integration.herdr", "", "existing Herdr 0.6.5 binary; otherwise use the verified test cache")
	pluginHerdrFlag     = flag.String("integration.plugin-herdr", "", "existing plugin-test Herdr binary; otherwise use verified Herdr 0.7.0")
	artifactsFlag       = flag.String("integration.artifacts", "dist/integration", "artifact directory, relative to the repository or absolute")
	sourceRefFlag       = flag.String("integration.source-ref", "", "published Git ref for the opt-in public plugin installation test")
	failAfterAttachFlag = flag.Bool("integration.fail-after-attach", false, "deliberately fail the lifecycle test after attaching, to verify cleanup")
	releaseTagFlag      = flag.String("integration.release-tag", "", "release tag to compare with the local plugin manifest")
	repo                string
	suiteContext        context.Context
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("HERDRCTX_TEST_HELPER"); mode != "" {
		os.Exit(runHelper(mode, os.Args[1:]))
	}
	flag.Parse()
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	repo, err = filepath.Abs(filepath.Join(cwd, ".."))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "integration tests require macOS or Linux")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	suiteContext = ctx
	code := m.Run()
	stop()
	os.Exit(code)
}

func repoPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(repo, path)
}

type scenario struct {
	t         *testing.T
	root      string
	artifacts string
	env       map[string]string
	metadata  map[string]any
	checks    []string
}

func newScenario(t *testing.T, name string) *scenario {
	t.Helper()
	// Long roots can exceed macOS's Unix-domain socket limit once Herdr adds a session name.
	root, err := os.MkdirTemp("/tmp", "hctx-")
	must(t, err)
	root, err = filepath.EvalSymlinks(root)
	must(t, err)
	must(t, os.MkdirAll(repoPath(*artifactsFlag), 0o700))
	artifacts, err := os.MkdirTemp(repoPath(*artifactsFlag), name+"-")
	must(t, err)
	s := &scenario{t: t, root: root, artifacts: artifacts, metadata: map[string]any{
		"test": t.Name(),
		"go":   runtime.Version(), "platform": runtime.GOOS + "/" + runtime.GOARCH,
		"binary": repoPath(*binaryFlag), "root": root,
	}}
	t.Logf("Artifacts: %s", artifacts)
	t.Cleanup(func() {
		if t.Failed() {
			s.snapshot()
		}
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove isolated root: %v", err)
		}
		s.metadata["passed"] = !t.Failed()
		s.metadata["checks"] = s.checks
		s.writeJSON("result.json", s.metadata)
	})
	for _, dir := range []string{"home", "config", "data", "state", "runtime", "work", "tmp", "bin"} {
		must(t, os.MkdirAll(filepath.Join(root, dir), 0o700))
	}
	s.env = map[string]string{
		"PATH": os.Getenv("PATH"), "HOME": filepath.Join(root, "home"),
		"XDG_CONFIG_HOME": filepath.Join(root, "config"), "XDG_DATA_HOME": filepath.Join(root, "data"),
		"XDG_STATE_HOME": filepath.Join(root, "state"), "XDG_RUNTIME_DIR": filepath.Join(root, "runtime"),
		"TMPDIR": filepath.Join(root, "tmp"), "TERM": "xterm-256color", "LANG": "en_US.UTF-8",
		"SHELL": "/bin/sh", "HERDR_DISABLE_SOUND": "1",
		"HERDR_CONFIG_PATH":  filepath.Join(root, "config", "config.toml"),
		"HERDRCTX_TEST_ROOT": root,
	}
	if raw, err := os.ReadFile(repoPath(*binaryFlag)); err == nil {
		s.metadata["binary_sha256"] = digest(raw)
	}
	return s
}

func (s *scenario) check(name string) {
	s.t.Helper()
	s.t.Log(name)
	s.checks = append(s.checks, name)
}

func (s *scenario) preferencesPath() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(s.env["HOME"], "Library", "Application Support", "herdrctx", "preferences.json")
	}
	return filepath.Join(s.env["XDG_CONFIG_HOME"], "herdrctx", "preferences.json")
}

func (s *scenario) writeJSON(name string, value any) {
	s.t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(s.artifacts, name), append(raw, '\n'), 0o600)
	}
	if err != nil {
		s.t.Errorf("write artifact %s: %v", name, err)
	}
}

func (s *scenario) snapshot() {
	s.t.Helper()
	var total int64
	err := filepath.WalkDir(s.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 16<<20 || total+info.Size() > 64<<20 {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(s.artifacts, "snapshot", rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		if err := copyFile(path, dest, 0o600); err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		s.t.Errorf("snapshot diagnostics: %v", err)
	}
}

func (s *scenario) command(timeout time.Duration, binary string, args ...string) (string, error) {
	return s.commandIn(timeout, s.root, binary, args...)
}

func (s *scenario) commandIn(timeout time.Duration, dir, binary string, args ...string) (string, error) {
	// Cleanup still needs a bounded command after the suite receives SIGTERM.
	base := suiteContext
	if base.Err() != nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, timeout)
	defer cancel()
	// #nosec G204 -- Run the test-selected executable directly, without a shell.
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir, cmd.Env = dir, environment(s.env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	raw, err := cmd.CombinedOutput()
	logErr := appendFile(filepath.Join(s.artifacts, "commands.log"),
		[]byte(fmt.Sprintf("%q %q: %v\n%s\n", binary, args, err, raw)))
	return string(raw), errors.Join(err, logErr)
}

func (s *scenario) cli(binary string, args ...string) string {
	s.t.Helper()
	output, err := s.command(15*time.Second, binary, args...)
	if err != nil {
		s.t.Fatalf("%s %q: %v\n%s", binary, args, err, output)
	}
	return output
}

func (s *scenario) wait(description string, predicate func() bool) {
	s.t.Helper()
	if err := waitFor(suiteContext, 20*time.Second, predicate); err != nil {
		s.t.Fatalf("%s: %v", description, err)
	}
}

func waitFor(ctx context.Context, timeout time.Duration, predicate func() bool) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if predicate() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("timed out after %s", timeout)
		case <-ticker.C:
		}
	}
}

func environment(values map[string]string) []string {
	env := make([]string, 0, len(values))
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	slices.Sort(env)
	return env
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	// #nosec G304 -- Callers select repository artifacts or files inside their test roots.
	raw, err := os.ReadFile(path)
	must(t, err)
	return raw
}

func writeFile(t *testing.T, path string, raw []byte, mode os.FileMode) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o700))
	must(t, os.WriteFile(path, raw, mode))
}

func copyFile(source, dest string, mode os.FileMode) error {
	// #nosec G304 G703 -- Copy only test-selected fixture files and binaries.
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	// #nosec G703 -- Callers supply destinations inside their isolated fixture directories.
	return os.WriteFile(dest, raw, mode)
}

func appendFile(path string, raw []byte) error {
	// #nosec G304 G703 -- Append only to logs selected by the isolated test harness.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(raw)
	return errors.Join(err, file.Close())
}

func atomicJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	must(t, err)
	temp, err := os.CreateTemp(filepath.Dir(path), ".fixture-*")
	must(t, err)
	// #nosec G703 -- Remove only the temporary fixture file created above.
	defer func() { _ = os.Remove(temp.Name()) }()
	_, err = temp.Write(raw)
	must(t, errors.Join(err, temp.Close()))
	// #nosec G703 -- Atomically replace a fixture within its isolated scenario directory.
	must(t, os.Rename(temp.Name(), path))
}

func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func killGroup(pid int, signal syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("invalid process group %d", pid)
	}
	err := syscall.Kill(-pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func contained(root, path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	// Resolve the nearest existing parent, including macOS /tmp symlinks.
	tail := []string{}
	parent := path
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			rel, err := filepath.Rel(root, resolved)
			return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(parent) == parent {
			return false
		}
		tail = append(tail, filepath.Base(parent))
		parent = filepath.Dir(parent)
	}
}

func expectContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("missing %q in:\n%s", want, text)
	}
}

func decodeJSON[T any](t *testing.T, raw string) T {
	t.Helper()
	var value T
	must(t, json.Unmarshal([]byte(raw), &value))
	return value
}
