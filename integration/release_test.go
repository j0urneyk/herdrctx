//go:build integration

package integration

import (
	"flag"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var manifestVersionLine = regexp.MustCompile(`^version\s*=\s*"([0-9A-Za-z.+-]+)"\s*(#.*)?$`)

func manifestVersion(raw string) (string, error) {
	var version string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			break
		}
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		key, _, hasEquals := strings.Cut(line, "=")
		if strings.TrimSpace(key) != "version" {
			continue
		}
		match := manifestVersionLine.FindStringSubmatch(line)
		if !hasEquals || match == nil || version != "" {
			return "", fmt.Errorf("expected one quoted top-level manifest version")
		}
		version = match[1]
	}
	if version == "" {
		return "", fmt.Errorf("missing top-level manifest version")
	}
	return version, nil
}

func TestManifestVersion(t *testing.T) {
	for _, raw := range []string{"version = \"0.0.3\"\n[[build]]\nversion = \"nested\"", "# comment\n version = \"0.0.3\" # comment\n"} {
		got, err := manifestVersion(raw)
		if err != nil || got != "0.0.3" {
			t.Fatalf("parse manifest: %q, %v", got, err)
		}
	}
	for _, raw := range []string{"", "[[build]]\nversion = \"nested\"", "version=123", "version=\"0.0.3\"\nversion=\"0.0.4\"", "version"} {
		if _, err := manifestVersion(raw); err == nil {
			t.Fatalf("accepted malformed version: %q", raw)
		}
	}
}

func TestReleaseVersion(t *testing.T) {
	if *releaseTagFlag == "" {
		specified := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "integration.release-tag" {
				specified = true
			}
		})
		if specified {
			t.Fatal("release tag must not be empty")
		}
		t.Skip("set -integration.release-tag to check a release tag")
	}
	version, err := manifestVersion(string(readFile(t, filepath.Join(repo, "herdr-plugin.toml"))))
	must(t, err)
	if *releaseTagFlag != "v"+version {
		t.Fatalf("release tag %q must match manifest version v%s", *releaseTagFlag, version)
	}
	t.Logf("Plugin manifest matches %s.", *releaseTagFlag)
}
