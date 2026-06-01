package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const DefaultDirectoryCompletionVisibleCount = 8

const (
	maxDirectoryCompletionScanned = 4096
	maxDirectoryCompletionItems   = 256
)

// CompletionHiddenMode controls when hidden directories appear in completion candidates.
type CompletionHiddenMode string

const (
	CompletionHiddenAuto   CompletionHiddenMode = "auto"
	CompletionHiddenAlways CompletionHiddenMode = "always"
	CompletionHiddenNever  CompletionHiddenMode = "never"
)

// ParseCompletionHiddenMode parses the user-facing hidden completion mode.
func ParseCompletionHiddenMode(value string) (CompletionHiddenMode, error) {
	mode := CompletionHiddenMode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case "", CompletionHiddenAuto:
		return CompletionHiddenAuto, nil
	case CompletionHiddenAlways, CompletionHiddenNever:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid hidden completion mode %q; use auto, always, or never", value)
	}
}

// ParseCompletionVisibleCount parses the user-facing completion row count.
func ParseCompletionVisibleCount(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultDirectoryCompletionVisibleCount, nil
	}

	count, err := strconv.Atoi(value)
	if err != nil || count <= 0 {
		return 0, fmt.Errorf("invalid completion count %q; use a positive integer", value)
	}

	return count, nil
}

func (m CompletionHiddenMode) withDefault() CompletionHiddenMode {
	if m == "" {
		return CompletionHiddenAuto
	}

	return m
}

type directoryCompletionOptions struct {
	HiddenMode   CompletionHiddenMode
	VisibleLimit int
}

func (o directoryCompletionOptions) withDefaults() directoryCompletionOptions {
	o.HiddenMode = o.HiddenMode.withDefault()
	if o.VisibleLimit <= 0 {
		o.VisibleLimit = DefaultDirectoryCompletionVisibleCount
	}

	return o
}

type directoryCompletionItem struct {
	Value string
}

type directoryCompletion struct {
	items        []directoryCompletionItem
	selected     int
	visibleLimit int
}

func newDirectoryCompletion(options directoryCompletionOptions) directoryCompletion {
	options = options.withDefaults()
	return directoryCompletion{visibleLimit: options.VisibleLimit}
}

func (c directoryCompletion) open() bool {
	return len(c.items) > 0
}

func (c *directoryCompletion) setItems(items []directoryCompletionItem) {
	c.items = items
	if len(c.items) == 0 {
		c.selected = 0
		return
	}
	if c.selected < 0 || c.selected >= len(c.items) {
		c.selected = 0
	}
}

func (c *directoryCompletion) next() {
	if !c.open() {
		return
	}
	c.selected++
	if c.selected >= len(c.items) {
		c.selected = 0
	}
}

func (c *directoryCompletion) prev() {
	if !c.open() {
		return
	}
	c.selected--
	if c.selected < 0 {
		c.selected = len(c.items) - 1
	}
}

func (c directoryCompletion) selectedItem() (directoryCompletionItem, bool) {
	if !c.open() || c.selected < 0 || c.selected >= len(c.items) {
		return directoryCompletionItem{}, false
	}

	return c.items[c.selected], true
}

func (c directoryCompletion) render(width int) string {
	if !c.open() {
		return ""
	}

	start, end := c.visibleRange()
	rowWidth := max(20, min(84, width-6))
	var b strings.Builder
	for i := start; i < end; i++ {
		item := c.items[i]
		prefix := "  "
		style := completionItemStyle
		if i == c.selected {
			prefix = "› "
			style = completionSelectedStyle
		}

		line := truncate(prefix+sanitizeDisplay(item.Value), rowWidth)
		b.WriteString(style.Width(rowWidth).Render(line))
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	if c.hasOverflow() {
		b.WriteString("\n")
		b.WriteString(subtleStyle.Width(rowWidth).Render(fmt.Sprintf("  %d-%d of %d", start+1, end, len(c.items))))
	}

	return b.String()
}

func (c directoryCompletion) renderedHeight() int {
	if !c.open() {
		return 0
	}

	start, end := c.visibleRange()
	height := end - start
	if c.hasOverflow() {
		height++
	}

	return height
}

func (c directoryCompletion) visibleRange() (int, int) {
	if !c.open() {
		return 0, 0
	}

	limit := c.effectiveVisibleLimit()
	if len(c.items) <= limit {
		return 0, len(c.items)
	}

	start := c.selected - limit + 1
	if start < 0 {
		start = 0
	}
	if start+limit > len(c.items) {
		start = len(c.items) - limit
	}

	return start, start + limit
}

func (c directoryCompletion) hasOverflow() bool {
	return len(c.items) > c.effectiveVisibleLimit()
}

func (c directoryCompletion) effectiveVisibleLimit() int {
	if c.visibleLimit <= 0 {
		return DefaultDirectoryCompletionVisibleCount
	}

	return c.visibleLimit
}

func directoryCompletionItems(input string, baseDir string, options directoryCompletionOptions) []directoryCompletionItem {
	options = options.withDefaults()
	location, ok := directoryCompletionLocationFor(input, baseDir)
	if !ok {
		return nil
	}

	dir, err := os.Open(location.searchDir)
	if err != nil {
		return nil
	}
	defer func() { _ = dir.Close() }()

	prefix := strings.ToLower(location.segment)
	items := make([]directoryCompletionItem, 0, min(options.VisibleLimit, maxDirectoryCompletionItems))
	scanned := 0
	for scanned < maxDirectoryCompletionScanned && len(items) < maxDirectoryCompletionItems {
		entries, err := dir.ReadDir(min(128, maxDirectoryCompletionScanned-scanned))
		if err != nil && !errors.Is(err, io.EOF) {
			return nil
		}
		scanned += len(entries)
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(strings.ToLower(name), prefix) {
				continue
			}
			if isHiddenName(name) && !includeHiddenCompletion(name, location.segment, options.HiddenMode) {
				continue
			}
			if !directoryEntryIsDir(location.searchDir, entry) {
				continue
			}

			value := location.displayPrefix + name + string(os.PathSeparator)
			if sanitizeDisplay(value) != value {
				continue
			}

			items = append(items, directoryCompletionItem{Value: value})
			if len(items) >= maxDirectoryCompletionItems {
				break
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}

	sort.Slice(items, func(i int, j int) bool {
		return strings.ToLower(items[i].Value) < strings.ToLower(items[j].Value)
	})

	return items
}

type directoryCompletionLocation struct {
	searchDir     string
	displayPrefix string
	segment       string
}

func directoryCompletionLocationFor(input string, baseDir string) (directoryCompletionLocation, bool) {
	if input == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return directoryCompletionLocation{}, false
		}
		return directoryCompletionLocation{searchDir: home, displayPrefix: "~/"}, true
	}

	if strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return directoryCompletionLocation{}, false
		}

		relativeInput := strings.TrimPrefix(input, "~/")
		parent, segment := splitCompletionInput(relativeInput)
		return directoryCompletionLocation{
			searchDir:     filepath.Join(home, parent),
			displayPrefix: "~/" + parent,
			segment:       segment,
		}, true
	}

	parent, segment := splitCompletionInput(input)
	searchDir := parent
	if !filepath.IsAbs(input) {
		if baseDir == "" {
			wd, err := os.Getwd()
			if err != nil {
				baseDir = "."
			} else {
				baseDir = wd
			}
		}
		searchDir = filepath.Join(baseDir, parent)
	}
	if searchDir == "" {
		searchDir = string(os.PathSeparator)
	}

	return directoryCompletionLocation{
		searchDir:     searchDir,
		displayPrefix: parent,
		segment:       segment,
	}, true
}

func splitCompletionInput(input string) (string, string) {
	index := strings.LastIndex(input, string(os.PathSeparator))
	if index < 0 {
		return "", input
	}

	return input[:index+1], input[index+1:]
}

func includeHiddenCompletion(name string, segment string, mode CompletionHiddenMode) bool {
	if !isHiddenName(name) {
		return true
	}

	switch mode.withDefault() {
	case CompletionHiddenAlways:
		return true
	case CompletionHiddenNever:
		return false
	default:
		return strings.HasPrefix(segment, ".")
	}
}

func isHiddenName(name string) bool {
	return strings.HasPrefix(name, ".") && name != "." && name != ".."
}

func directoryEntryIsDir(parent string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}

	// #nosec G703 -- entry.Name comes from os.ReadDir for parent during completion.
	info, err := os.Stat(filepath.Join(parent, entry.Name()))
	return err == nil && info.IsDir()
}
