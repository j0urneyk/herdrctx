package herdr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	defaultCommandTimeout       = 15 * time.Second
	defaultCommandWaitDelay     = time.Second
	maxCommandOutputBytes       = 64 * 1024
	maxCommandErrorPreviewBytes = 4 * 1024
	maxSessionListOutputBytes   = 4 * 1024 * 1024
)

var errCommandOutputLimit = errors.New("command output exceeded capture limit")

// Client shells out to the Herdr CLI.
type Client struct {
	Bin       string
	Timeout   time.Duration
	WaitDelay time.Duration
}

// CommandError includes command output and Herdr's structured error, when available.
type CommandError struct {
	Args         []string
	ExitCode     int
	Stdout       string
	Stderr       string
	HerdrCode    string
	HerdrMessage string
	Err          error
}

func (e *CommandError) Error() string {
	command := "herdr"
	if len(e.Args) > 0 {
		command = strings.Join(e.Args, " ")
	}

	message := e.HerdrMessage
	if message == "" {
		message = commandOutputPreview(e.Stderr)
	}
	if message == "" {
		message = commandOutputPreview(e.Stdout)
	}
	if message == "" && e.Err != nil {
		message = e.Err.Error()
	}
	if message == "" {
		message = "command failed"
	}

	if e.HerdrCode != "" {
		return fmt.Sprintf("%s: %s (%s)", command, message, e.HerdrCode)
	}

	return fmt.Sprintf("%s: %s", command, message)
}

func commandOutputPreview(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= maxCommandErrorPreviewBytes {
		return output
	}

	preview := strings.TrimSpace(output[:maxCommandErrorPreviewBytes])
	return fmt.Sprintf("%s\n[output preview truncated after %d bytes]", preview, maxCommandErrorPreviewBytes)
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

// NewClient creates a Herdr CLI client.
func NewClient(bin string) *Client {
	if strings.TrimSpace(bin) == "" {
		bin = "herdr"
	}

	return &Client{Bin: bin, Timeout: defaultCommandTimeout, WaitDelay: defaultCommandWaitDelay}
}

// ListSessions returns the current Herdr session list.
func (c *Client) ListSessions(ctx context.Context) ([]Session, error) {
	stdout, err := c.runWithOutputLimit(ctx, maxSessionListOutputBytes, "session", "list", "--json")
	if err != nil {
		return nil, err
	}

	return ParseSessions(stdout)
}

// StopSession stops a running Herdr session.
func (c *Client) StopSession(ctx context.Context, name string) error {
	name, err := ValidateSessionName(name)
	if err != nil {
		return err
	}
	if IsJSONFlagName(name) {
		return fmt.Errorf("session name %q cannot be stopped by Herdr", name)
	}

	_, err = c.run(ctx, "session", "stop", name, "--json")
	return err
}

// DeleteSession deletes a stopped Herdr session.
func (c *Client) DeleteSession(ctx context.Context, name string) error {
	name, err := ValidateSessionName(name)
	if err != nil {
		return err
	}
	if IsJSONFlagName(name) {
		return fmt.Errorf("session name %q cannot be deleted by Herdr", name)
	}

	_, err = c.run(ctx, "session", "delete", name, "--json")
	return err
}

// AttachCommand returns the foreground attach command for a Herdr session.
func (c *Client) AttachCommand(name string) (*exec.Cmd, error) {
	name, err := ValidateSessionName(name)
	if err != nil {
		return nil, err
	}
	if IsAttachHelpName(name) {
		return nil, fmt.Errorf("session name %q cannot be attached by Herdr", name)
	}

	// #nosec G204 -- The configured Herdr binary is the intended integration boundary.
	return exec.Command(c.Bin, "session", "attach", name), nil
}

// CreateAttachCommand returns the foreground command that uses or creates a named session.
func (c *Client) CreateAttachCommand(name string, startDir string) (*exec.Cmd, error) {
	name, err := ValidateNewSessionName(name)
	if err != nil {
		return nil, err
	}

	startDir, err = ResolveStartDir(startDir, "")
	if err != nil {
		return nil, err
	}

	// #nosec G204 -- The configured Herdr binary is the intended integration boundary.
	cmd := exec.Command(c.Bin, "--session", name)
	cmd.Dir = startDir
	return cmd, nil
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	return c.runWithOutputLimit(ctx, maxCommandOutputBytes, args...)
}

func (c *Client) runWithOutputLimit(ctx context.Context, outputLimit int, args ...string) ([]byte, error) {
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	// #nosec G204 -- The configured Herdr binary is the intended integration boundary.
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return killCommandGroup(cmd)
	}
	if c.WaitDelay > 0 {
		cmd.WaitDelay = c.WaitDelay
	}

	stdout := newCappedBuffer(outputLimit)
	stderr := newCappedBuffer(maxCommandOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		_ = killCommandGroup(cmd)
	}
	if err == nil && (stdout.truncated || stderr.truncated) {
		err = errCommandOutputLimit
	}
	if err == nil {
		return stdout.Bytes(), nil
	}

	return nil, c.commandError(ctx, args, stdout.String(), stderr.String(), err)
}

func killCommandGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}

	return err
}

type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newCappedBuffer(limit int) *cappedBuffer {
	return &cappedBuffer{limit: limit}
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}

	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}

	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *cappedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

func (b *cappedBuffer) String() string {
	value := b.buf.String()
	if !b.truncated {
		return value
	}
	if value != "" && !strings.HasSuffix(value, "\n") {
		value += "\n"
	}

	return fmt.Sprintf("%s[output truncated after %d bytes]", value, b.limit)
}

func (c *Client) commandError(ctx context.Context, args []string, stdout string, stderr string, err error) error {
	exitCode := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	herdrCode, herdrMessage := parseErrorResponse([]byte(stderr))
	if herdrMessage == "" {
		herdrCode, herdrMessage = parseErrorResponse([]byte(stdout))
	}
	if herdrMessage == "" && errors.Is(err, errCommandOutputLimit) {
		herdrMessage = errCommandOutputLimit.Error()
	}
	if herdrMessage == "" && errors.Is(err, exec.ErrWaitDelay) {
		herdrMessage = "command I/O wait timed out"
	}

	if ctx.Err() != nil {
		err = ctx.Err()
		if herdrMessage == "" {
			herdrMessage = "command timed out"
		}
	}

	fullArgs := append([]string{c.Bin}, args...)

	return &CommandError{
		Args:         fullArgs,
		ExitCode:     exitCode,
		Stdout:       stdout,
		Stderr:       stderr,
		HerdrCode:    herdrCode,
		HerdrMessage: herdrMessage,
		Err:          err,
	}
}
