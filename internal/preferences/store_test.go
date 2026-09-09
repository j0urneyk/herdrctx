package preferences

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFavoriteChangesPreserveOtherInstances(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "config", "preferences.json")}
	d, err := s.Load()
	if err != nil || d.Version != 1 {
		t.Fatalf("load: %v %v", d, err)
	}
	other := &Store{Path: s.Path}
	if _, err = other.Apply(Change{FavoriteName: "web", Favorite: true}); err != nil {
		t.Fatal(err)
	}
	d, err = s.Apply(Change{FavoriteName: "api", Favorite: true})
	if err != nil || !d.Favorite("web") || !d.Favorite("api") {
		t.Fatalf("lost other instance's favorite: %+v, %v", d, err)
	}
	d, err = other.Apply(Change{FavoriteName: "web", Favorite: false})
	if err != nil || d.Favorite("web") || !d.Favorite("api") {
		t.Fatalf("removal changed another favorite: %+v, %v", d, err)
	}
	lock, err := lockFile(s.Path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(Change{FavoriteName: "api", Favorite: false}); err == nil {
		t.Fatal("lock not enforced")
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatal(info.Mode())
	}
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["version"] == nil || fields["favorites"] == nil {
		t.Fatalf("unexpected saved fields: %s", raw)
	}
}

func TestInvalidPreferencesRemainUntouched(t *testing.T) {
	for _, raw := range []string{`{`, `{"version":2}`, `{"version":1,"unknown":true}`, `{"version":1} {}`, `{"version":1,"favorites":["api","api"]}`} {
		t.Run(raw, func(t *testing.T) {
			s := &Store{Path: filepath.Join(t.TempDir(), "preferences.json")}
			if err := os.WriteFile(s.Path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Apply(Change{FavoriteName: "api", Favorite: true}); err == nil {
				t.Fatal("accepted invalid file")
			}
			got, err := os.ReadFile(s.Path)
			if err != nil || string(got) != raw {
				t.Fatal("changed original file")
			}
		})
	}
}

func TestWriteFailurePreservesExistingData(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: dir}
	if err := s.write([]byte("test")); err == nil {
		t.Fatal("replaced directory")
	}
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".preferences-") {
			t.Fatal("temporary file leaked")
		}
	}
}
