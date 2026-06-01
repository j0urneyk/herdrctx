package herdr

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const MinimumSupportedVersion = "0.6.5"

var (
	minimumSupportedSemanticVersion = semanticVersion{major: 0, minor: 6, patch: 5, raw: MinimumSupportedVersion}
	versionPattern                  = regexp.MustCompile(`\b[vV]?([0-9]+)\.([0-9]+)\.([0-9]+)(?:[-+][0-9A-Za-z.-]+)?\b`)
)

type semanticVersion struct {
	major int
	minor int
	patch int
	raw   string
}

func (c *Client) EnsureMinimumVersion(ctx context.Context) error {
	stdout, err := c.run(ctx, "--version")
	if err != nil {
		return fmt.Errorf("herdrctx requires herdr %s or newer; check Herdr version: %w", MinimumSupportedVersion, err)
	}

	found, err := parseVersionOutput(stdout)
	if err != nil {
		return fmt.Errorf("herdrctx requires herdr %s or newer; %w", MinimumSupportedVersion, err)
	}
	if found.compare(minimumSupportedSemanticVersion) < 0 {
		return fmt.Errorf("herdrctx requires herdr %s or newer; found %s", MinimumSupportedVersion, found)
	}

	return nil
}

func parseVersionOutput(output []byte) (semanticVersion, error) {
	match := versionPattern.FindStringSubmatch(string(output))
	if match == nil {
		return semanticVersion{}, fmt.Errorf("could not read Herdr version from %q", strings.TrimSpace(string(output)))
	}

	major, err := strconv.Atoi(match[1])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("read Herdr major version: %w", err)
	}
	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("read Herdr minor version: %w", err)
	}
	patch, err := strconv.Atoi(match[3])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("read Herdr patch version: %w", err)
	}

	return semanticVersion{major: major, minor: minor, patch: patch, raw: strings.TrimPrefix(strings.TrimPrefix(match[0], "v"), "V")}, nil
}

func (v semanticVersion) compare(other semanticVersion) int {
	switch {
	case v.major != other.major:
		return compareInt(v.major, other.major)
	case v.minor != other.minor:
		return compareInt(v.minor, other.minor)
	case v.patch != other.patch:
		return compareInt(v.patch, other.patch)
	default:
		return 0
	}
}

func (v semanticVersion) String() string {
	return v.raw
}

func compareInt(left int, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
