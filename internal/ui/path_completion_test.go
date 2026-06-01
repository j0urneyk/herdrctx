package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDirectoryCompletionItemsUseRelativePaths(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "projects"))
	mustMkdir(t, filepath.Join(base, "prototypes"))
	mustWriteFile(t, filepath.Join(base, "profile.txt"))

	items := directoryCompletionItems("pro", base, directoryCompletionOptions{VisibleLimit: 8})
	values := completionValues(items)
	want := []string{"projects/", "prototypes/"}
	if !equalStrings(values, want) {
		t.Fatalf("values = %#v, want %#v", values, want)
	}
}

func TestDirectoryCompletionItemsPreserveRelativeParent(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "workspaces", "herdrctx"))

	items := directoryCompletionItems("workspaces/h", base, directoryCompletionOptions{VisibleLimit: 8})
	values := completionValues(items)
	want := []string{"workspaces/herdrctx/"}
	if !equalStrings(values, want) {
		t.Fatalf("values = %#v, want %#v", values, want)
	}
}

func TestDirectoryCompletionItemsUseAbsolutePaths(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	parent := filepath.Join(base, "parent")
	child := filepath.Join(parent, "child")
	mustMkdir(t, child)

	items := directoryCompletionItems(filepath.Join(parent, "c"), base, directoryCompletionOptions{VisibleLimit: 8})
	values := completionValues(items)
	want := []string{child + string(os.PathSeparator)}
	if !equalStrings(values, want) {
		t.Fatalf("values = %#v, want %#v", values, want)
	}
}

func TestDirectoryCompletionItemsIncludeSymlinkedDirectories(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	target := filepath.Join(base, "target")
	mustMkdir(t, target)
	if err := os.Symlink(target, filepath.Join(base, "linked")); err != nil {
		t.Fatalf("symlink directory: %v", err)
	}

	items := directoryCompletionItems("lin", base, directoryCompletionOptions{VisibleLimit: 8})
	values := completionValues(items)
	want := []string{"linked/"}
	if !equalStrings(values, want) {
		t.Fatalf("values = %#v, want %#v", values, want)
	}
}

func TestDirectoryCompletionItemsSkipControlByteNames(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "work\x1b[31m"))
	mustMkdir(t, filepath.Join(base, "work-safe"))

	items := directoryCompletionItems("work", base, directoryCompletionOptions{VisibleLimit: 8})
	values := completionValues(items)
	want := []string{"work-safe/"}
	if !equalStrings(values, want) {
		t.Fatalf("values = %#v, want %#v", values, want)
	}
}

func TestDirectoryCompletionHiddenModes(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, ".config"))
	mustMkdir(t, filepath.Join(base, "code"))

	autoItems := directoryCompletionItems("", base, directoryCompletionOptions{HiddenMode: CompletionHiddenAuto, VisibleLimit: 8})
	if slices.Contains(completionValues(autoItems), ".config/") {
		t.Fatalf("auto empty-prefix values include hidden directory: %#v", completionValues(autoItems))
	}

	autoDotItems := directoryCompletionItems(".", base, directoryCompletionOptions{HiddenMode: CompletionHiddenAuto, VisibleLimit: 8})
	if !slices.Contains(completionValues(autoDotItems), ".config/") {
		t.Fatalf("auto dot-prefix values omit hidden directory: %#v", completionValues(autoDotItems))
	}

	alwaysItems := directoryCompletionItems("", base, directoryCompletionOptions{HiddenMode: CompletionHiddenAlways, VisibleLimit: 8})
	if !slices.Contains(completionValues(alwaysItems), ".config/") {
		t.Fatalf("always values omit hidden directory: %#v", completionValues(alwaysItems))
	}

	neverItems := directoryCompletionItems(".", base, directoryCompletionOptions{HiddenMode: CompletionHiddenNever, VisibleLimit: 8})
	if slices.Contains(completionValues(neverItems), ".config/") {
		t.Fatalf("never values include hidden directory: %#v", completionValues(neverItems))
	}
}

func TestDirectoryCompletionItemsIgnoreMissingParents(t *testing.T) {
	t.Parallel()

	items := directoryCompletionItems("missing/child", t.TempDir(), directoryCompletionOptions{VisibleLimit: 8})
	if len(items) != 0 {
		t.Fatalf("items = %#v, want none", items)
	}
}

func TestDirectoryCompletionItemsDoNotTruncateMatches(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	for i := range 10 {
		mustMkdir(t, filepath.Join(base, fmt.Sprintf("dir%02d", i)))
	}

	items := directoryCompletionItems("dir", base, directoryCompletionOptions{VisibleLimit: 8})
	if len(items) != 10 {
		t.Fatalf("len(items) = %d, want 10", len(items))
	}
}

func TestDirectoryCompletionItemsAreBounded(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	for i := range maxDirectoryCompletionItems + 20 {
		mustMkdir(t, filepath.Join(base, fmt.Sprintf("dir%03d", i)))
	}

	items := directoryCompletionItems("dir", base, directoryCompletionOptions{VisibleLimit: 8})
	if len(items) > maxDirectoryCompletionItems {
		t.Fatalf("len(items) = %d, want at most %d", len(items), maxDirectoryCompletionItems)
	}
}

func TestDirectoryCompletionRenderUsesVisibleWindow(t *testing.T) {
	t.Parallel()

	completion := newDirectoryCompletion(directoryCompletionOptions{VisibleLimit: 8})
	completion.setItems(tenCompletionItems())
	for range 8 {
		completion.next()
	}

	view := completion.render(80)
	if strings.Contains(view, "dir00/") {
		t.Fatalf("render includes first candidate outside visible window:\n%s", view)
	}
	if !strings.Contains(view, "› dir08/") {
		t.Fatalf("render missing selected candidate in visible window:\n%s", view)
	}
	if !strings.Contains(view, "2-9 of 10") {
		t.Fatalf("render missing overflow count:\n%s", view)
	}
	if height := completion.renderedHeight(); height != 9 {
		t.Fatalf("renderedHeight = %d, want 9", height)
	}
}

func TestDirectoryCompletionRenderSanitizesControlSequences(t *testing.T) {
	t.Parallel()

	completion := newDirectoryCompletion(directoryCompletionOptions{VisibleLimit: 8})
	completion.setItems([]directoryCompletionItem{{Value: "bad\x1b]52;c;secret\adir/"}})

	view := completion.render(80)
	if strings.Contains(view, "\x1b]52") || strings.Contains(view, "secret") {
		t.Fatalf("render contains control sequence: %q", view)
	}
	if !strings.Contains(view, "baddir/") {
		t.Fatalf("render = %q, want sanitized candidate", view)
	}
	item, ok := completion.selectedItem()
	if !ok {
		t.Fatal("selectedItem() ok = false")
	}
	if item.Value != "bad\x1b]52;c;secret\adir/" {
		t.Fatalf("selected raw value = %q, want original", item.Value)
	}
}

func TestDirectoryCompletionNavigationWrapsAllItems(t *testing.T) {
	t.Parallel()

	completion := newDirectoryCompletion(directoryCompletionOptions{VisibleLimit: 8})
	completion.setItems(tenCompletionItems())
	completion.prev()
	item, ok := completion.selectedItem()
	if !ok {
		t.Fatal("selectedItem() ok = false")
	}
	if item.Value != "dir09/" {
		t.Fatalf("selected after prev = %q, want dir09/", item.Value)
	}

	completion.next()
	item, ok = completion.selectedItem()
	if !ok {
		t.Fatal("selectedItem() ok = false")
	}
	if item.Value != "dir00/" {
		t.Fatalf("selected after wrap next = %q, want dir00/", item.Value)
	}
}

func TestParseCompletionHiddenMode(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "auto", "always", "never", "AUTO"} {
		if _, err := ParseCompletionHiddenMode(value); err != nil {
			t.Fatalf("ParseCompletionHiddenMode(%q) error = %v", value, err)
		}
	}
	if _, err := ParseCompletionHiddenMode("sometimes"); err == nil {
		t.Fatal("ParseCompletionHiddenMode() error = nil, want error")
	}
}

func TestParseCompletionVisibleCount(t *testing.T) {
	t.Parallel()

	count, err := ParseCompletionVisibleCount("")
	if err != nil {
		t.Fatalf("ParseCompletionVisibleCount(empty) error = %v", err)
	}
	if count != DefaultDirectoryCompletionVisibleCount {
		t.Fatalf("default count = %d, want %d", count, DefaultDirectoryCompletionVisibleCount)
	}

	count, err = ParseCompletionVisibleCount("12")
	if err != nil {
		t.Fatalf("ParseCompletionVisibleCount(12) error = %v", err)
	}
	if count != 12 {
		t.Fatalf("count = %d, want 12", count)
	}

	for _, value := range []string{"0", "-1", "many"} {
		if _, err := ParseCompletionVisibleCount(value); err == nil {
			t.Fatalf("ParseCompletionVisibleCount(%q) error = nil, want error", value)
		}
	}
}

func tenCompletionItems() []directoryCompletionItem {
	items := make([]directoryCompletionItem, 0, 10)
	for i := range 10 {
		items = append(items, directoryCompletionItem{Value: fmt.Sprintf("dir%02d/", i)})
	}

	return items
}

func completionValues(items []directoryCompletionItem) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, item.Value)
	}

	return values
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
