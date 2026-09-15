//go:build integration

package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/j0urneyk/herdrctx/internal/preferences"
)

func (s *scenario) legacyHerdrctx() string {
	s.t.Helper()
	platform := runtime.GOOS + "/" + runtime.GOARCH
	digests := map[string]string{
		"darwin/arm64": "0e0dab4188055c974f6518c4b0226ceabe3e376330921b7c46a46dd822ca82b6",
		"darwin/amd64": "4466a4c142382f41aa960f7377c7e9993234c4c911cf509dcf131f7c7d251466",
		"linux/arm64":  "f36202f6dbec99fe91d68ef8ff18e9787f337b24a4ca779f0cdc5b2d4730b54d",
		"linux/amd64":  "c2967e54f0242ee074fcfba8b940b7e285e1a439ada8d25e4c3827af7126bc65",
	}
	if digests[platform] == "" {
		s.t.Fatalf("unsupported legacy test platform %s", platform)
	}
	asset := fmt.Sprintf("herdrctx_0.0.4_%s_%s.tar.gz", map[string]string{"darwin": "macos", "linux": "linux"}[runtime.GOOS], map[string]string{"arm64": "aarch64", "amd64": "x86_64"}[runtime.GOARCH])
	cache := filepath.Join(repo, "bin", asset)
	// #nosec G304 -- The cache path is built from a pinned release and supported platform names.
	raw, err := os.ReadFile(cache)
	if os.IsNotExist(err) {
		raw, err = download(suiteContext, "https://github.com/j0urneyk/herdrctx/releases/download/v0.0.4/"+asset)
		must(s.t, err)
		if digest(raw) != digests[platform] {
			s.t.Fatal("legacy archive checksum mismatch")
		}
		writeFile(s.t, cache, raw, 0o600)
	}
	must(s.t, err)
	if digest(raw) != digests[platform] {
		s.t.Fatal("cached legacy archive checksum mismatch")
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	must(s.t, err)
	defer func() { _ = gz.Close() }()
	archive := tar.NewReader(io.LimitReader(gz, 32<<20))
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		must(s.t, err)
		if header.Name != "herdrctx" {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size > 20<<20 {
			s.t.Fatal("unexpected legacy binary archive entry")
		}
		binary, err := io.ReadAll(archive)
		must(s.t, err)
		path := filepath.Join(s.root, "bin", "legacy-herdrctx")
		writeFile(s.t, path, binary, 0o700)
		version := strings.TrimSpace(s.cli(path, "--version"))
		if version != "herdrctx 0.0.4" {
			s.t.Fatalf("unexpected legacy version %q", version)
		}
		s.metadata["legacy_version"] = version
		s.metadata["legacy_archive_sha256"] = digest(raw)
		s.metadata["legacy_binary_sha256"] = digest(binary)
		return path
	}
	s.t.Fatal("legacy archive lacks herdrctx")
	return ""
}

func TestMixedVersionPreferences(t *testing.T) {
	n := newRemoteNavigation(t)
	s := n.s
	legacy := s.legacyHerdrctx()
	localHerdr := s.fixture("herdr")
	atomicJSON(t, filepath.Join(s.root, "sessions.json"), sessionList{Sessions: n.state.Sessions})
	writeFile(t, s.preferencesPath(), []byte(`{"version":1,"favorites":["api"]}`), 0o600)
	old := s.terminal("legacy-open-before-migration", legacy, "--herdr-bin", localHerdr, "--interval", "500ms")
	old.expect("Loaded 1 session(s).", 0)
	old.sendExpect("i", "Favorite: true")
	old.sendExpect(enter, "enter/a")
	current := n.start("current-migrates")
	current.send("p")
	store := &preferences.Store{Path: s.preferencesPath()}
	s.wait("v2 migration while old app is running", func() bool {
		d, err := store.Load()
		return err == nil && d.Version == 2 && d.Favorite("api") && len(d.RemoteFavorites) == 1
	})
	before := readFile(t, s.preferencesPath())
	old.sendExpect("p", "Preferences unavailable")
	if !bytes.Equal(before, readFile(t, s.preferencesPath())) {
		t.Fatal("old app overwrote migrated preferences")
	}
	old.sendExpect(enter, "enter/a")
	old.quit()
	old = s.terminal("legacy-started-after-migration", legacy, "--herdr-bin", localHerdr, "--interval", "500ms")
	old.expect("Preferences unavailable", 0)
	old.sendExpect(enter, "enter/a")
	old.sendExpect("p", "Preferences unavailable")
	if !bytes.Equal(before, readFile(t, s.preferencesPath())) {
		t.Fatal("old startup changed v2 preferences")
	}
	old.sendExpect(enter, "enter/a")
	old.sendExpect("n", "New session")
	old.sendExpect(esc, "enter/a")
	current.send("p")
	s.wait("current app keeps writing", func() bool {
		d, err := store.Load()
		return err == nil && d.Version == 2 && d.Favorite("api") && len(d.RemoteFavorites) == 0
	})
	current.quit()
	old.quit()
	restarted := n.start("current-restarted")
	d, err := store.Load()
	must(t, err)
	if !d.Favorite("api") || len(d.RemoteFavorites) != 0 || d.Version != 2 {
		t.Fatalf("bad persisted preferences: %+v", d)
	}
	restarted.quit()
	s.check("v0.0.4 and current apps coexist without overwriting v2 or losing local favorites")
}
