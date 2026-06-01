package herdr

import "strings"

// InsideHerdr reports whether the process appears to be running inside a Herdr-managed pane.
func InsideHerdr(env []string) bool {
	return envValue(env, "HERDR_ENV") == "1" || envValue(env, "HERDR_SOCKET_PATH") != ""
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}

	return ""
}
