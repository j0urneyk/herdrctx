package herdr

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const MinimumRemoteVersion = "0.8.2"

// RemoteTarget is an explicit OpenSSH destination. Its spelling is its identity;
// aliases are not resolved into hostnames when saving preferences.
type RemoteTarget struct{ Destination string }

func ParseRemoteTarget(value string) (*RemoteTarget, error) {
	if value == "" || len(value) > 1024 || strings.TrimSpace(value) != value || strings.HasPrefix(value, "-") {
		return nil, fmt.Errorf("remote target must be an SSH host, user@host, or ssh://user@host:port")
	}
	for _, r := range value {
		allowed := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-@:/[]%", r)
		if !allowed {
			return nil, fmt.Errorf("remote target contains an unsupported character")
		}
	}
	if strings.HasPrefix(value, "ssh://") {
		u, err := url.Parse(value)
		if err != nil || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("invalid SSH URL")
		}
		if u.User != nil {
			if _, set := u.User.Password(); set || u.User.Username() == "" {
				return nil, fmt.Errorf("SSH URL must not contain a password or empty user")
			}
		}
		if u.Port() != "" {
			port, err := strconv.Atoi(u.Port())
			if err != nil || port < 1 || port > 65535 {
				return nil, fmt.Errorf("SSH port must be between 1 and 65535")
			}
		}
	} else {
		if strings.ContainsAny(value, "/:[]%") || strings.Count(value, "@") > 1 {
			return nil, fmt.Errorf("use ssh://user@host:port for a port or IPv6 address")
		}
		for part := range strings.SplitSeq(value, "@") {
			if part == "" || strings.HasPrefix(part, "-") {
				return nil, fmt.Errorf("invalid SSH host or user")
			}
		}
	}
	return &RemoteTarget{Destination: value}, nil
}

func (c *Client) TargetID() string {
	if c.Remote == nil {
		return ""
	}
	return c.Remote.Destination
}

// OutcomeUnknownError means the remote command may have run; it must not be replayed.
type OutcomeUnknownError struct {
	Target, Action, Name string
	Err                  error
}

func (e *OutcomeUnknownError) Error() string {
	return fmt.Sprintf("Could not confirm %s of %q on %s. The remote operation may have completed. Refresh to check its current state before trying again.\n\n%v", e.Action, e.Name, e.Target, e.Err)
}
func (e *OutcomeUnknownError) Unwrap() error { return e.Err }

func quoteRemote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func (c *Client) remoteCommand(ctx context.Context, limit int, command string) ([]byte, error) {
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2", "--", c.Remote.Destination, command}
	return c.executeWithOutputLimit(ctx, limit, "ssh", args...)
}

func (c *Client) remoteList(ctx context.Context) ([]Session, error) {
	raw, err := c.remoteCommand(ctx, maxSessionListOutputBytes, "herdr --version && herdr session list --json")
	if err != nil {
		return nil, fmt.Errorf("list %s (check SSH authentication and remote Herdr on PATH): %w", c.TargetID(), err)
	}
	version, jsonText, ok := strings.Cut(string(raw), "\n")
	if !ok {
		return nil, fmt.Errorf("remote Herdr returned no session JSON")
	}
	if err := checkRemoteVersion([]byte(version)); err != nil {
		return nil, err
	}
	sessions, err := ParseSessions([]byte(jsonText))
	for i := range sessions {
		sessions[i].Target = c.TargetID()
	}
	return sessions, err
}

func (c *Client) runSessionMutation(ctx context.Context, action, name string) error {
	if c.Remote == nil {
		_, err := c.run(ctx, "session", action, name, "--json")
		return err
	}
	_, err := c.remoteCommand(ctx, maxCommandOutputBytes, "herdr session "+action+" "+quoteRemote(name)+" --json")
	if err == nil {
		return nil
	}
	var commandErr *CommandError
	if errors.As(err, &commandErr) && commandErr.HerdrCode != "" {
		return err
	}
	return &OutcomeUnknownError{Target: c.TargetID(), Action: action, Name: name, Err: err}
}

func checkRemoteVersion(raw []byte) error {
	v, err := parseVersionOutput(raw)
	if err != nil {
		return err
	}
	if v.compare(semanticVersion{major: 0, minor: 8, patch: 2}) < 0 {
		return fmt.Errorf("remote mode requires Herdr %s or newer on both hosts; found %s", MinimumRemoteVersion, v)
	}
	return nil
}
