//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var testDigests = map[string]map[string]string{
	"0.6.5": {
		"linux/amd64":  "70ef4ce425c0697901a26b6c07562faf0d7f54d8c6b6df542a95a9774760e2bf",
		"linux/arm64":  "78d5e27b335ae656218f2a23d355e9ccab0db32dcd85bec91945eb9acd7d8669",
		"darwin/amd64": "174ea85c099b1fbe8f217759959a7365a9a631d6d74d81b6a6bab642a7309444",
		"darwin/arm64": "0938c67cc1c11762cf20ebc993be96d12c0fff784edc649c708d79ae8c67e8da",
	},
	"0.7.0": {
		"linux/amd64":  "ad2a5d480a4e04609a9dd30a19ec07854578df6b5f0ea9299246963baf40363b",
		"linux/arm64":  "77407959c514c25c870bbcc6d2a2c86fef5b5701ed0c7c37745d7412e8563d72",
		"darwin/amd64": "6c61cdb67c79b8d0626e109b9d8d8635c66a80bfed21ac9fe6efdf1dd8d27c0f",
		"darwin/arm64": "0946c1c5de396d1404906c81c84a0cef47af5e15c9aac3c058c3936b833fe311",
	},
}

func (s *scenario) herdr(version, override string) string {
	s.t.Helper()
	if override != "" {
		path := repoPath(override)
		got := strings.TrimSpace(s.cli(path, "--version"))
		if version == "0.6.5" && got != "herdr "+version {
			s.t.Fatalf("expected Herdr %s, got %q", version, got)
		}
		s.metadata["herdr"] = got
		return path
	}
	filename := "herdr-ci"
	if version == "0.7.0" {
		filename = "herdr-plugin-ci"
	}
	path := filepath.Join(repo, "bin", filename)
	target := runtime.GOOS + "/" + runtime.GOARCH
	expected := testDigests[version][target]
	if expected == "" {
		s.t.Fatalf("unsupported Herdr test target %s", target)
	}
	// #nosec G304 -- This is one of the two fixed binary cache paths in the repository.
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		asset := "herdr-" + map[string]string{"darwin": "macos", "linux": "linux"}[runtime.GOOS] + "-" + map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
		url := "https://github.com/herdrdev/herdr/releases/download/v" + version + "/" + asset
		s.t.Logf("Downloading Herdr %s for %s", version, target)
		raw, err = download(suiteContext, url)
		must(s.t, err)
		if digest(raw) != expected {
			s.t.Fatalf("SHA-256 mismatch for %s", url)
		}
		must(s.t, os.MkdirAll(filepath.Dir(path), 0o700))
		temp, err := os.CreateTemp(filepath.Dir(path), ".herdr-test-*")
		must(s.t, err)
		// #nosec G703 -- Remove only the temporary download created above.
		defer func() { _ = os.Remove(temp.Name()) }()
		_, err = temp.Write(raw)
		if closeErr := temp.Close(); err == nil {
			err = closeErr
		}
		must(s.t, err)
		// #nosec G302 G703 -- Make only our verified temporary download executable.
		must(s.t, os.Chmod(temp.Name(), 0o700))
		if got := strings.TrimSpace(s.cli(temp.Name(), "--version")); got != "herdr "+version {
			s.t.Fatalf("unexpected downloaded binary %q", got)
		}
		// #nosec G703 -- Both paths belong to the fixed test binary cache.
		must(s.t, os.Rename(temp.Name(), path))
	} else {
		must(s.t, err)
		if digest(raw) != expected {
			s.t.Fatalf("cached Herdr %s has an unexpected SHA-256: %s", version, path)
		}
	}
	got := strings.TrimSpace(s.cli(path, "--version"))
	if got != "herdr "+version {
		s.t.Fatalf("expected Herdr %s, got %q", version, got)
	}
	s.metadata["herdr"] = got
	s.metadata["herdr_sha256"] = expected
	return path
}

func download(ctx context.Context, url string) ([]byte, error) {
	client := &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing non-HTTPS redirect")
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many download redirects")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// #nosec G704 -- The URL is constructed from pinned Herdr versions and target names.
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, res.Status)
	}
	const limit = 100 << 20
	raw, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > limit {
		return nil, fmt.Errorf("download exceeds %d bytes", limit)
	}
	return raw, nil
}
