package herdr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveStartDir expands and validates a session start directory.
func ResolveStartDir(input string, baseDir string) (string, error) {
	return resolveStartDir(input, baseDir, false)
}

// ResolveOrCreateStartDir expands a session start directory and creates it when missing.
func ResolveOrCreateStartDir(input string, baseDir string) (string, error) {
	return resolveStartDir(input, baseDir, true)
}

func resolveStartDir(input string, baseDir string, createMissing bool) (string, error) {
	resolved, err := resolveStartDirPath(input, baseDir)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if !createMissing || !os.IsNotExist(err) {
			return "", fmt.Errorf("start directory %q is not available: %w", resolved, err)
		}
		if err := os.MkdirAll(resolved, 0o750); err != nil {
			return "", fmt.Errorf("create start directory %q: %w", resolved, err)
		}
		info, err = os.Stat(resolved)
		if err != nil {
			return "", fmt.Errorf("start directory %q is not available: %w", resolved, err)
		}
	}
	if !info.IsDir() {
		return "", fmt.Errorf("start directory %q is not a directory", resolved)
	}

	return resolved, nil
}

func resolveStartDirPath(input string, baseDir string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("start directory is required")
	}
	if containsPathControl(input) {
		return "", fmt.Errorf("start directory cannot contain control characters")
	}

	expanded, err := expandHome(input)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(expanded) {
		if baseDir == "" {
			baseDir, err = os.Getwd()
			if err != nil {
				return "", fmt.Errorf("get working directory: %w", err)
			}
		}
		expanded = filepath.Join(baseDir, expanded)
	}

	resolved, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve start directory: %w", err)
	}

	return resolved, nil
}

func containsPathControl(value string) bool {
	return strings.ContainsFunc(value, func(char rune) bool {
		return char < 0x20 || char == 0x7f || (char >= 0x80 && char <= 0x9f)
	})
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}

	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}
