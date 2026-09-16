package preferences

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/j0urneyk/herdrctx/internal/herdr"
)

// Host stores connection metadata only. SSH owns credentials and host resolution.
type Host struct {
	Label       string `json:"label"`
	Destination string `json:"destination"`
	ProfileID   string `json:"profile_id,omitempty"`
	Session     string `json:"session,omitempty"`
}
type Hosts struct {
	Version int    `json:"version"`
	Items   []Host `json:"hosts"`
}
type (
	HostStore  struct{ Path string }
	HostChange struct {
		Previous string
		Remove   bool
		Host     Host
	}
)

func DefaultHostStore() (*HostStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &HostStore{Path: filepath.Join(dir, "herdrctx", "hosts.json")}, nil
}

func (d Hosts) validate() error {
	if d.Version != 1 {
		return fmt.Errorf("unsupported hosts version %d", d.Version)
	}
	seen := map[string]bool{}
	labels := map[string]bool{}
	profiles := map[string]bool{}
	for _, h := range d.Items {
		if h.Label == "" || strings.TrimSpace(h.Label) != h.Label || len(h.Label) > 80 {
			return fmt.Errorf("host label must contain 1–80 bytes without surrounding whitespace")
		}
		for _, r := range h.Label {
			if r < 32 || r == 127 {
				return fmt.Errorf("host label contains a control character")
			}
		}
		if _, err := herdr.ParseRemoteTarget(h.Destination); err != nil {
			return err
		}
		if h.Session != "" {
			if _, err := herdr.ValidateSessionName(h.Session); err != nil {
				return err
			}
		}
		if seen[h.Destination] || labels[h.Label] || h.ProfileID != "" && profiles[h.ProfileID] {
			return fmt.Errorf("duplicate host destination, label, or profile")
		}
		seen[h.Destination] = true
		labels[h.Label] = true
		profiles[h.ProfileID] = true
	}
	return nil
}

func (s *HostStore) Load() (Hosts, error) {
	raw, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Hosts{Version: 1, Items: []Host{}}, nil
	}
	if err != nil {
		return Hosts{}, err
	}
	var d Hosts
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&d); err != nil {
		return Hosts{}, err
	}
	var tail any
	if err = dec.Decode(&tail); !errors.Is(err, io.EOF) {
		return Hosts{}, fmt.Errorf("unexpected trailing host settings data")
	}
	return d, d.validate()
}

func (s *HostStore) Apply(c HostChange) (Hosts, error) {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return Hosts{}, err
	}
	lock, err := lockFile(s.Path + ".lock")
	if err != nil {
		return Hosts{}, err
	}
	defer func() { _ = lock.Close() }()
	d, err := s.Load()
	if err != nil {
		return Hosts{}, err
	}
	index := slices.IndexFunc(d.Items, func(h Host) bool { return h.Destination == c.Previous })
	if c.Previous != "" && index < 0 {
		return Hosts{}, fmt.Errorf("host changed in another instance; restart herdrctx to reload hosts")
	}
	switch {
	case c.Remove:
		if index < 0 {
			return Hosts{}, fmt.Errorf("no host selected")
		}
		d.Items = slices.Delete(d.Items, index, index+1)
	case index >= 0:
		d.Items[index] = c.Host
	default:
		d.Items = append(d.Items, c.Host)
	}
	if err = d.validate(); err != nil {
		return Hosts{}, err
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return Hosts{}, err
	}
	if err = (&Store{Path: s.Path}).write(append(raw, '\n')); err != nil {
		return Hosts{}, err
	}
	return d, nil
}
