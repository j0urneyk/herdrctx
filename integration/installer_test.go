//go:build integration

package integration

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type installer struct {
	s           *scenario
	checkout    string
	manifest    string
	downloads   string
	destination string
	log         string
}

func newInstaller(t *testing.T) *installer {
	t.Helper()
	s := newScenario(t, "installer")
	i := &installer{s: s, checkout: filepath.Join(s.root, "checkout with spaces"), downloads: filepath.Join(s.root, "downloads"), destination: filepath.Join(s.root, "installed bin", "herdrctx"), log: filepath.Join(s.root, "urls.txt")}
	i.manifest = filepath.Join(i.checkout, "herdr-plugin.toml")
	writeFile(t, filepath.Join(i.checkout, "scripts", "install.sh"), readFile(t, filepath.Join(repo, "scripts", "install.sh")), 0o700)
	i.version("0.0.2")
	must(t, os.MkdirAll(i.downloads, 0o700))
	s.env["PATH"] = filepath.Join(s.root, "bin") + string(os.PathListSeparator) + s.env["PATH"]
	s.env["HERDRCTX_INSTALL_DIR"] = filepath.Dir(i.destination)
	s.env["FIXTURE_DIR"] = i.downloads
	s.env["FIXTURE_LOG"] = i.log
	s.env["FIXTURE_OS"], s.env["FIXTURE_ARCH"] = "Linux", "x86_64"
	s.fixture("uname")
	s.fixture("curl")
	return i
}

func (i *installer) version(version string) {
	writeFile(i.s.t, i.manifest, []byte(fmt.Sprintf("version = %q\n[[build]]\ncommand = [\"sh\", \"scripts/install.sh\"]\n", version)), 0o600)
}

func (i *installer) fixture(version, target, member string, content []byte) (string, string) {
	i.s.t.Helper()
	directory := filepath.Join(i.downloads, "v"+version)
	must(i.s.t, os.MkdirAll(directory, 0o700))
	asset := filepath.Join(directory, fmt.Sprintf("herdrctx_%s_%s.tar.gz", version, target))
	if content == nil {
		content = []byte(fmt.Sprintf("#!/bin/sh\nprintf 'herdrctx %s\\n'\n", version))
	}
	// #nosec G304 -- Asset paths are generated under this test's temporary download directory.
	file, err := os.Create(asset)
	must(i.s.t, err)
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	err = archive.WriteHeader(&tar.Header{Name: member, Mode: 0o755, Size: int64(len(content))})
	if err == nil {
		_, err = archive.Write(content)
	}
	must(i.s.t, errors.Join(err, archive.Close(), compressed.Close(), file.Close()))
	checksums := filepath.Join(directory, "checksums.txt")
	must(i.s.t, appendFile(checksums, []byte(digest(readFile(i.s.t, asset))+"  "+filepath.Base(asset)+"\n")))
	return asset, checksums
}

func (i *installer) install(success bool) string {
	i.s.t.Helper()
	output, err := i.s.command(20*time.Second, "sh", filepath.Join(i.checkout, "scripts", "install.sh"))
	if (err == nil) != success {
		i.s.t.Fatalf("install success=%v, error=%v\n%s", success, err, output)
	}
	if success {
		info, err := os.Stat(i.destination)
		must(i.s.t, err)
		if info.Mode().Perm()&0o111 == 0 {
			i.s.t.Fatal("installed binary is not executable")
		}
	}
	staged, err := filepath.Glob(filepath.Join(filepath.Dir(i.destination), ".herdrctx.*"))
	must(i.s.t, err)
	if len(staged) != 0 {
		i.s.t.Fatalf("staged files remain: %v", staged)
	}
	temp, err := os.ReadDir(i.s.env["TMPDIR"])
	must(i.s.t, err)
	if len(temp) != 0 {
		i.s.t.Fatalf("temporary files remain: %v", temp)
	}
	return output
}

func TestInstallerPlatforms(t *testing.T) {
	for _, platform := range []struct{ system, arch, target string }{
		{"Darwin", "x86_64", "macos_x86_64"},
		{"Darwin", "arm64", "macos_aarch64"},
		{"Linux", "x86_64", "linux_x86_64"},
		{"Linux", "aarch64", "linux_aarch64"},
	} {
		t.Run(platform.target, func(t *testing.T) {
			i := newInstaller(t)
			i.s.env["FIXTURE_OS"], i.s.env["FIXTURE_ARCH"] = platform.system, platform.arch
			for _, version := range []string{"0.0.2", "1.2.3-rc.1"} {
				i.fixture(version, platform.target, "herdrctx", nil)
				i.version(version)
				for range 2 {
					expectContains(t, i.install(true), "Installed herdrctx "+version)
					expectContains(t, string(readFile(t, i.log)), fmt.Sprintf("/v%s/herdrctx_%s_%s.tar.gz", version, version, platform.target))
					if got := strings.TrimSpace(i.s.cli(i.destination, "--version")); got != "herdrctx "+version {
						t.Fatalf("installed version: %q", got)
					}
				}
			}
		})
	}
}

func TestInstallerFailurePreservesBinary(t *testing.T) {
	for _, failure := range []string{"download", "checksum download", "missing checksum", "duplicate checksum", "checksum mismatch", "invalid archive", "missing binary"} {
		t.Run(failure, func(t *testing.T) {
			i := newInstaller(t)
			asset, checksums := i.fixture("0.0.2", "linux_x86_64", "herdrctx", nil)
			originalChecksums := readFile(t, checksums)
			writeFile(t, i.destination, []byte("existing binary"), 0o700)
			switch failure {
			case "download":
				i.s.env["FIXTURE_FAIL"] = "1"
			case "checksum download":
				must(t, os.Remove(checksums))
			case "missing checksum":
				writeFile(t, checksums, []byte(strings.ReplaceAll(string(originalChecksums), filepath.Base(asset), "another.tar.gz")), 0o600)
			case "duplicate checksum":
				writeFile(t, checksums, append(originalChecksums, originalChecksums...), 0o600)
			case "checksum mismatch":
				writeFile(t, checksums, []byte(strings.Repeat("0", 64)+"  "+filepath.Base(asset)+"\n"), 0o600)
			case "invalid archive":
				writeFile(t, asset, []byte("invalid tar"), 0o600)
				writeFile(t, checksums, []byte(digest(readFile(t, asset))+"  "+filepath.Base(asset)+"\n"), 0o600)
			case "missing binary":
				must(t, os.Remove(checksums))
				i.fixture("0.0.2", "linux_x86_64", "README.md", nil)
			}
			expectContains(t, i.install(false), "herdrctx install:")
			if string(readFile(t, i.destination)) != "existing binary" {
				t.Fatal("failed install overwrote existing binary")
			}
		})
	}
}

func TestInstallerInvalidManifest(t *testing.T) {
	for index, manifest := range []string{
		"", `version = "latest"`, `version = "../1.2.3"`, `version = "01.2.3"`,
		`version = "1.2.3-01"`, `version = "1.2.3-rc.01"`,
		"version = \"1.2.3\"\nversion = \"1.2.4\"", "version = 123", "[[build]]\nversion = \"1.2.3\"",
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			i := newInstaller(t)
			writeFile(t, i.manifest, []byte(manifest), 0o600)
			expectContains(t, i.install(false), "version")
			if _, err := os.Stat(i.log); !os.IsNotExist(err) {
				t.Fatal("invalid manifest attempted a download")
			}
		})
	}
	t.Run("missing", func(t *testing.T) {
		i := newInstaller(t)
		must(t, os.Remove(i.manifest))
		expectContains(t, i.install(false), "Missing manifest")
	})
}

func TestInstallerUnsupportedPlatform(t *testing.T) {
	i := newInstaller(t)
	i.s.env["FIXTURE_OS"] = "FreeBSD"
	expectContains(t, i.install(false), "Supported platforms")
	if _, err := os.Stat(i.log); !os.IsNotExist(err) {
		t.Fatal("unsupported platform attempted a download")
	}
}

func TestInstallerUnwritableDestination(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission denial requires an unprivileged user")
	}
	i := newInstaller(t)
	must(t, os.MkdirAll(filepath.Dir(i.destination), 0o700))
	// #nosec G302 -- Directory traversal bits are needed to test a read-only installation directory.
	must(t, os.Chmod(filepath.Dir(i.destination), 0o555))
	// #nosec G302 -- Restore owner traversal before removing the test directory.
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(i.destination), 0o700) })
	expectContains(t, i.install(false), "not writable")
	if _, err := os.Stat(i.log); !os.IsNotExist(err) {
		t.Fatal("unwritable destination attempted a download")
	}
}

func TestInstallerPathAdvice(t *testing.T) {
	i := newInstaller(t)
	i.fixture("0.0.2", "linux_x86_64", "herdrctx", nil)
	shadow := filepath.Join(i.s.root, "bin", "herdrctx")
	writeFile(t, shadow, []byte("#!/bin/sh\nexit 0\n"), 0o700)
	output := i.install(true)
	expectContains(t, output, "Add "+filepath.Dir(i.destination)+" to PATH")
	expectContains(t, output, "PATH currently selects "+shadow)
	i.s.env["PATH"] = filepath.Dir(i.destination) + string(os.PathListSeparator) + i.s.env["PATH"]
	output = i.install(true)
	if strings.Contains(output, "Add ") || strings.Contains(output, "PATH currently selects") {
		t.Fatal("unnecessary PATH advice")
	}
}

func TestInstallerNativeBinary(t *testing.T) {
	i := newInstaller(t)
	system, arch := "Linux", "x86_64"
	osName, cpu := "linux", "x86_64"
	if runtime.GOOS == "darwin" {
		system, osName = "Darwin", "macos"
	}
	if runtime.GOARCH == "arm64" {
		arch, cpu = "arm64", "aarch64"
	}
	i.s.env["FIXTURE_OS"], i.s.env["FIXTURE_ARCH"] = system, arch
	i.fixture("0.0.2", osName+"_"+cpu, "herdrctx", readFile(t, repoPath(*binaryFlag)))
	i.install(true)
	expected := i.s.cli(repoPath(*binaryFlag), "--version")
	if got := i.s.cli(i.destination, "--version"); got != expected {
		t.Fatalf("version changed: %q", got)
	}
	expectContains(t, i.s.cli(i.destination, "--help"), "Usage: herdrctx [flags]")
}
