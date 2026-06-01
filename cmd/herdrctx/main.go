package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/j0urneyk/herdrctx/internal/herdr"
	"github.com/j0urneyk/herdrctx/internal/ui"
)

var version = "dev"

const minimumRefreshInterval = 500 * time.Millisecond

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	herdrBinDefault := os.Getenv("HERDRCTX_HERDR_BIN")
	if herdrBinDefault == "" {
		herdrBinDefault = os.Getenv("HERDR_BIN")
	}
	if herdrBinDefault == "" {
		herdrBinDefault = "herdr"
	}

	completeHiddenDefault := os.Getenv("HERDRCTX_COMPLETE_HIDDEN")
	if completeHiddenDefault == "" {
		completeHiddenDefault = string(ui.CompletionHiddenAuto)
	}
	completeCountDefault := os.Getenv("HERDRCTX_COMPLETE_COUNT")
	if completeCountDefault == "" {
		completeCountDefault = fmt.Sprint(ui.DefaultDirectoryCompletionVisibleCount)
	}

	flags := flag.NewFlagSet("herdrctx", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	interval := flags.Duration("interval", 3*time.Second, "session refresh interval (minimum 500ms)")
	herdrBin := flags.String("herdr-bin", herdrBinDefault, "path to the herdr binary")
	allowNested := flags.Bool("allow-nested", truthyEnv("HERDRCTX_ALLOW_NESTED"), "allow launching Herdr from inside Herdr")
	completeHidden := flags.String("complete-hidden", completeHiddenDefault, "hidden directory completion mode: auto, always, or never")
	completeCount := flags.String("complete-count", completeCountDefault, "number of visible directory completion rows")
	showVersion := flags.Bool("version", false, "print version and exit")

	flags.Usage = func() {
		_, _ = fmt.Fprintf(flags.Output(), "Usage: %s [flags]\n\n", flags.Name())
		_, _ = fmt.Fprintln(flags.Output(), "Interactive Herdr session manager.")
		_, _ = fmt.Fprintf(flags.Output(), "Requires Herdr %s or newer.\n", herdr.MinimumSupportedVersion)
		_, _ = fmt.Fprintln(flags.Output(), "")
		_, _ = fmt.Fprintln(flags.Output(), "Flags:")
		flags.PrintDefaults()
	}

	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if *showVersion {
		_, _ = fmt.Printf("herdrctx %s\n", version)
		return nil
	}
	if *interval <= 0 {
		return fmt.Errorf("--interval must be positive")
	}
	if *interval < minimumRefreshInterval {
		return fmt.Errorf("--interval must be at least %s", minimumRefreshInterval)
	}
	hiddenMode, err := ui.ParseCompletionHiddenMode(*completeHidden)
	if err != nil {
		return err
	}
	visibleCount, err := ui.ParseCompletionVisibleCount(*completeCount)
	if err != nil {
		return err
	}
	if *herdrBin == "" {
		return fmt.Errorf("--herdr-bin must not be empty")
	}
	if os.Getenv("TERM") == "dumb" {
		return fmt.Errorf("herdrctx needs an interactive terminal; TERM=dumb is not supported")
	}
	if !fileDescriptorIsTerminal(os.Stdin) || !fileDescriptorIsTerminal(os.Stdout) {
		return fmt.Errorf("herdrctx needs an interactive TTY on stdin and stdout")
	}

	client := herdr.NewClient(*herdrBin)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := client.EnsureMinimumVersion(ctx); err != nil {
		return err
	}

	program := tea.NewProgram(ui.NewModel(ui.Options{
		Client:               client,
		Context:              ctx,
		Cancel:               cancel,
		RefreshInterval:      *interval,
		Version:              version,
		CompleteHidden:       hiddenMode,
		CompleteVisibleCount: visibleCount,
		InsideHerdr:          herdr.InsideHerdr(os.Environ()),
		CurrentSocketPath:    os.Getenv("HERDR_SOCKET_PATH"),
		AllowNested:          *allowNested,
	}))

	_, err = program.Run()
	return err
}

func truthyEnv(key string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func fileDescriptorIsTerminal(file *os.File) bool {
	fd := file.Fd()
	maxInt := uintptr(^uint(0) >> 1)
	if fd > maxInt {
		return false
	}

	// #nosec G115 -- The descriptor is bounded before converting to the API type.
	return term.IsTerminal(int(fd))
}
