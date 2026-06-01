package main

import (
	"os"
	"strings"
	"testing"
)

func TestRunHelpReturnsNil(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"herdrctx", "-h"}

	if err := run(); err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
}

func TestRunRejectsTooSmallInterval(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"herdrctx", "--interval", "1ns"}

	err := run()
	if err == nil {
		t.Fatal("run() error = nil, want interval error")
	}
	if !strings.Contains(err.Error(), "at least 500ms") {
		t.Fatalf("run() error = %q, want minimum interval", err.Error())
	}
}
