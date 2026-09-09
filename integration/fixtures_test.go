//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type session struct {
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	Default    bool   `json:"default"`
	SessionDir string `json:"session_dir"`
	SocketPath string `json:"socket_path"`
}

type sessionList struct {
	Sessions []session `json:"sessions"`
	Fail     bool      `json:"fail_list,omitempty"`
}

type invocation struct {
	Args []string `json:"argv"`
	CWD  string   `json:"cwd"`
}

func (s *scenario) fixture(name string) string {
	executable, err := os.Executable()
	must(s.t, err)
	path := filepath.Join(s.root, "bin", name)
	writeFile(s.t, path, []byte("#!/bin/sh\nHERDRCTX_TEST_HELPER="+quote(name)+" exec "+quote(executable)+" \"$@\"\n"), 0o700)
	return path
}

func runHelper(mode string, args []string) int {
	var err error
	switch mode {
	case "herdr":
		err = fixtureHerdr(args)
	case "curl":
		err = fixtureCurl(args)
	case "uname":
		switch {
		case slices.Equal(args, []string{"-s"}):
			fmt.Println(os.Getenv("FIXTURE_OS"))
		case slices.Equal(args, []string{"-m"}):
			fmt.Println(os.Getenv("FIXTURE_ARCH"))
		default:
			err = fmt.Errorf("unexpected uname arguments %q", args)
		}
	default:
		err = fmt.Errorf("unknown fixture %q", mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 22
	}
	return 0
}

func fixtureHerdr(args []string) error {
	if slices.Equal(args, []string{"--version"}) {
		fmt.Println("herdr 0.6.5")
		return nil
	}
	root := os.Getenv("HERDRCTX_TEST_ROOT")
	// #nosec G304 G703 -- The parent test supplies this isolated fixture directory.
	raw, err := os.ReadFile(filepath.Join(root, "sessions.json"))
	if err != nil {
		return err
	}
	var state sessionList
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if slices.Equal(args, []string{"session", "list", "--json"}) {
		if state.Fail {
			return fmt.Errorf(`{"error":{"code":"fixture_offline","message":"navigation fixture offline"}}`)
		}
		_, err := os.Stdout.Write(raw)
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	action, err := json.Marshal(invocation{Args: args, CWD: cwd})
	if err != nil {
		return err
	}
	if err := appendFile(filepath.Join(root, "actions.jsonl"), append(action, '\n')); err != nil {
		return err
	}
	if len(args) == 3 && slices.Equal(args[:2], []string{"session", "attach"}) {
		fmt.Println("NAVIGATION_ATTACH_HANDOFF")
		return nil
	}
	return fmt.Errorf("unexpected session action: %q", args)
}

func fixtureCurl(args []string) error {
	i := slices.Index(args, "-o")
	if i < 1 || i+1 >= len(args) {
		return fmt.Errorf("unexpected curl arguments %q", args)
	}
	url, dest := args[i-1], args[i+1]
	if err := appendFile(os.Getenv("FIXTURE_LOG"), []byte(url+"\n")); err != nil {
		return err
	}
	const prefix = "https://github.com/j0urneyk/herdrctx/releases/download/"
	if !strings.HasPrefix(url, prefix) {
		return fmt.Errorf("unexpected download URL %q", url)
	}
	if os.Getenv("FIXTURE_FAIL") != "" {
		return fmt.Errorf("injected download failure")
	}
	source := filepath.Join(os.Getenv("FIXTURE_DIR"), strings.TrimPrefix(url, prefix))
	return copyFile(source, dest, 0o600)
}
