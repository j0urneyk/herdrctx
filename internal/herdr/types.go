package herdr

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxSessionNameBytes = 64

// Session is one entry from `herdr session list --json`.
type Session struct {
	Default    bool   `json:"default"`
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	SessionDir string `json:"session_dir"`
	SocketPath string `json:"socket_path"`
}

// Status returns the short display status for a session.
func (s Session) Status() string {
	if s.Running {
		return "running"
	}

	return "stopped"
}

// DisplayName adds the default marker without changing the Herdr session name.
func (s Session) DisplayName() string {
	if s.Default {
		return s.Name + " *"
	}

	return s.Name
}

type listResponse struct {
	Sessions []Session `json:"sessions"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// ParseSessions parses the JSON output from `herdr session list --json`.
func ParseSessions(data []byte) ([]Session, error) {
	var response listResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("parse session list JSON: %w", err)
	}
	seenNames := make(map[string]struct{}, len(response.Sessions))
	for i := range response.Sessions {
		name := response.Sessions[i].Name
		if err := validateParsedSessionName(name); err != nil {
			return nil, err
		}
		if _, ok := seenNames[name]; ok {
			return nil, fmt.Errorf("duplicate session name %q in session list", name)
		}
		seenNames[name] = struct{}{}
	}

	return response.Sessions, nil
}

func validateParsedSessionName(name string) error {
	normalized, err := ValidateSessionName(name)
	if err != nil {
		return fmt.Errorf("invalid session name %q in session list: %w", name, err)
	}
	if normalized != name {
		return fmt.Errorf("invalid session name %q in session list: session name cannot contain leading or trailing whitespace", name)
	}

	return nil
}

func parseErrorResponse(data []byte) (code string, message string) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "", ""
	}

	var response errorResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		return "", ""
	}

	return response.Error.Code, response.Error.Message
}

func IsAttachHelpName(name string) bool {
	return name == "help" || name == "-h" || name == "--help"
}

func IsJSONFlagName(name string) bool {
	return name == "--json"
}

// ValidateSessionName returns a session name or an error.
func ValidateSessionName(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("session name is required")
	}
	if strings.TrimSpace(name) != name {
		return "", fmt.Errorf("session name cannot contain leading or trailing whitespace")
	}
	if len(name) > maxSessionNameBytes {
		return "", fmt.Errorf("session name must be at most %d bytes", maxSessionNameBytes)
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("session name cannot be %q", name)
	}
	for _, char := range name {
		if isSessionNameChar(char) {
			continue
		}

		return "", fmt.Errorf("session name can only contain ASCII letters, numbers, '.', '_', and '-'")
	}

	return name, nil
}

func isSessionNameChar(char rune) bool {
	return char >= 'a' && char <= 'z' ||
		char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' ||
		char == '.' ||
		char == '_' ||
		char == '-'
}
