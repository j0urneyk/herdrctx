package herdr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Machine struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Target   string `json:"target"`
	Session  string `json:"session"`
	Enabled  bool   `json:"enabled"`
	Selected bool   `json:"selected"`
}

func (c *Client) Machines(ctx context.Context) ([]Machine, error) {
	raw, err := c.run(ctx, "--version")
	if err != nil {
		return nil, err
	}
	v, err := parseVersionOutput(raw)
	if err != nil {
		return nil, err
	}
	if v.compare(semanticVersion{minor: 9}) < 0 {
		return nil, fmt.Errorf("machine import requires local Herdr 0.9.0 or newer")
	}
	raw, err = c.runWithOutputLimit(ctx, maxSessionListOutputBytes, "machine", "list", "--json")
	if err != nil {
		return nil, err
	}
	return ParseMachines(raw)
}

func ParseMachines(raw []byte) ([]Machine, error) {
	var rows []Machine
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("parse machine list: %w", err)
	}
	if rows == nil {
		return nil, fmt.Errorf("machine list must be a JSON array")
	}
	seen := map[string]bool{}
	for _, m := range rows {
		if m.ID == "" || m.Label == "" || len(m.Label) > 80 || strings.TrimSpace(m.Label) != m.Label || strings.ContainsFunc(m.Label, func(r rune) bool { return r < 32 || r == 127 }) || seen[m.ID] {
			return nil, fmt.Errorf("invalid or duplicate machine profile")
		}
		seen[m.ID] = true
		if _, err := ParseRemoteTarget(m.Target); err != nil {
			return nil, err
		}
		if _, err := ValidateSessionName(m.Session); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
