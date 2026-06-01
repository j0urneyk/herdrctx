package herdr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStartDir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	child := filepath.Join(base, "child")
	if err := os.Mkdir(child, 0o750); err != nil {
		t.Fatalf("mkdir child: %v", err)
	}

	resolved, err := ResolveStartDir("child", base)
	if err != nil {
		t.Fatalf("ResolveStartDir() error = %v", err)
	}
	if resolved != child {
		t.Fatalf("resolved = %q, want %q", resolved, child)
	}
}

func TestResolveStartDirRejectsEmpty(t *testing.T) {
	t.Parallel()

	if _, err := ResolveStartDir(" ", t.TempDir()); err == nil {
		t.Fatal("ResolveStartDir() error = nil, want error")
	}
}

func TestResolveStartDirPreservesSurroundingWhitespace(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	dir := filepath.Join(base, " project ")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatalf("mkdir whitespace dir: %v", err)
	}

	resolved, err := ResolveStartDir(" project ", base)
	if err != nil {
		t.Fatalf("ResolveStartDir() error = %v", err)
	}
	if resolved != dir {
		t.Fatalf("resolved = %q, want %q", resolved, dir)
	}
}

func TestResolveStartDirRejectsControlCharacters(t *testing.T) {
	t.Parallel()

	if _, err := ResolveStartDir("work\x1b[31m", t.TempDir()); err == nil {
		t.Fatal("ResolveStartDir() error = nil, want control-character error")
	}
}

func TestResolveStartDirRejectsMissingDirectory(t *testing.T) {
	t.Parallel()

	if _, err := ResolveStartDir("missing", t.TempDir()); err == nil {
		t.Fatal("ResolveStartDir() error = nil, want error")
	}
}

func TestResolveStartDirRejectsFile(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := ResolveStartDir(file, base); err == nil {
		t.Fatal("ResolveStartDir() error = nil, want error")
	}
}

func TestResolveOrCreateStartDirCreatesMissingDirectory(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	dir := filepath.Join(base, "new", "child")

	resolved, err := ResolveOrCreateStartDir(filepath.Join("new", "child"), base)
	if err != nil {
		t.Fatalf("ResolveOrCreateStartDir() error = %v", err)
	}
	if resolved != dir {
		t.Fatalf("resolved = %q, want %q", resolved, dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat created directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("created path is not a directory")
	}
}

func TestResolveOrCreateStartDirRejectsEmpty(t *testing.T) {
	t.Parallel()

	if _, err := ResolveOrCreateStartDir(" ", t.TempDir()); err == nil {
		t.Fatal("ResolveOrCreateStartDir() error = nil, want error")
	}
}

func TestResolveOrCreateStartDirRejectsFile(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := ResolveOrCreateStartDir(file, base); err == nil {
		t.Fatal("ResolveOrCreateStartDir() error = nil, want error")
	}
}
