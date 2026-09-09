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

	"github.com/j0urneyk/herdrctx/internal/herdr"
)

type Data struct {
	Version   int      `json:"version"`
	Favorites []string `json:"favorites"`
}

func Empty() Data                        { return Data{Version: 1, Favorites: []string{}} }
func (d Data) Favorite(name string) bool { return slices.Contains(d.Favorites, name) }
func (d Data) validate() error {
	if d.Version != 1 {
		return fmt.Errorf("unsupported preferences version %d", d.Version)
	}
	seen := map[string]bool{}
	for _, name := range d.Favorites {
		if _, err := herdr.ValidateSessionName(name); err != nil {
			return err
		}
		if seen[name] {
			return fmt.Errorf("duplicate favorite %q", name)
		}
		seen[name] = true
	}
	return nil
}

type Store struct{ Path string }

func DefaultStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{Path: filepath.Join(dir, "herdrctx", "preferences.json")}, nil
}

func (s *Store) Load() (Data, error) {
	raw, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return Data{}, err
	}
	var d Data
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return Data{}, err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Data{}, fmt.Errorf("unexpected trailing preferences data")
	}
	if err := d.validate(); err != nil {
		return Data{}, err
	}
	return d, nil
}

// Change carries only the edited item, preserving changes made by other instances.
type Change struct {
	FavoriteName string
	Favorite     bool
}

func (s *Store) Apply(c Change) (Data, error) {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return Data{}, err
	}
	lock, err := lockFile(s.Path + ".lock")
	if err != nil {
		return Data{}, err
	}
	defer func() { _ = lock.Close() }()
	// Reload under the lock; atomic replacement alone cannot prevent lost updates.
	d, err := s.Load()
	if err != nil {
		return Data{}, err
	}
	if c.FavoriteName != "" {
		d.Favorites = slices.DeleteFunc(d.Favorites, func(n string) bool { return n == c.FavoriteName })
		if c.Favorite {
			d.Favorites = append(d.Favorites, c.FavoriteName)
		}
	}

	if err := d.validate(); err != nil {
		return Data{}, err
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return Data{}, err
	}
	if err := s.write(append(raw, '\n')); err != nil {
		return Data{}, err
	}
	return s.Load()
}

func (s *Store) write(raw []byte) error {
	f, err := os.CreateTemp(filepath.Dir(s.Path), ".preferences-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		// #nosec G703 -- Remove only the temporary file created in our preferences directory.
		_ = os.Remove(f.Name())
	}()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// #nosec G703 -- Both paths are controlled by the configured preferences store.
	return os.Rename(f.Name(), s.Path)
}
