package preferences

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/j0urneyk/herdrctx/internal/herdr"
)

func TestRemoteFavoritesMigrateAndPreserveEachHost(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "preferences.json")}
	if err := os.WriteFile(s.Path, []byte(`{"version":1,"favorites":["api"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"workbox", "user@other"} {
		if _, err := s.Apply(Change{Target: target, FavoriteName: "api", Favorite: true}); err != nil {
			t.Fatal(err)
		}
	}
	d, err := s.Load()
	if err != nil || d.Version != 2 || !d.Favorite("api") || len(d.RemoteFavorites) != 2 {
		t.Fatalf("migration %+v %v", d, err)
	}
	other := &Store{Path: s.Path}
	if _, err = other.Apply(Change{Target: "workbox", FavoriteName: "api", Favorite: false}); err != nil {
		t.Fatal(err)
	}
	d, err = s.Apply(Change{FavoriteName: "web", Favorite: true})
	if err != nil || d.FavoriteSession(herdr.SessionID{Target: "workbox", Name: "api"}) || !d.FavoriteSession(herdr.SessionID{Target: "user@other", Name: "api"}) || !d.Favorite("api") || !d.Favorite("web") {
		t.Fatalf("cross-host update %+v %v", d, err)
	}
	before, _ := os.ReadFile(s.Path)
	lock, err := lockFile(s.Path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	_, saveErr := s.Apply(Change{Target: "workbox", FavoriteName: "api", Favorite: true})
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.Path)
	if saveErr == nil || string(before) != string(after) {
		t.Fatal("failed save changed data")
	}
}

func TestInvalidRemotePreferencesArePreserved(t *testing.T) {
	for _, raw := range []string{
		`{"version":1,"remote_favorites":[{"target":"host","name":"api"}]}`,
		`{"version":2,"remote_favorites":[{"target":"-bad","name":"api"}]}`,
		`{"version":2,"remote_favorites":[{"target":"host","name":"bad name"}]}`,
		`{"version":2,"remote_favorites":[{"target":"host","name":"api"},{"target":"host","name":"api"}]}`,
	} {
		s := &Store{Path: filepath.Join(t.TempDir(), "preferences.json")}
		if err := os.WriteFile(s.Path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(Change{Target: "host", FavoriteName: "api", Favorite: true}); err == nil {
			t.Fatal("accepted invalid data")
		}
		after, _ := os.ReadFile(s.Path)
		if string(after) != raw {
			t.Fatal("overwrote invalid file")
		}
	}
}
