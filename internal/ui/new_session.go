package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/j0urneyk/herdrctx/internal/herdr"
)

type newSessionMode int

const (
	newSessionQuick newSessionMode = iota
	newSessionWithDir
)

type newSessionField int

const (
	newSessionNameField newSessionField = iota
	newSessionDirField
)

type newSessionForm struct {
	mode                    newSessionMode
	active                  newSessionField
	defaultDir              string
	completionOptions       directoryCompletionOptions
	completion              directoryCompletion
	completionSuppressedFor string
	name                    textinput.Model
	dir                     textinput.Model
	err                     string
}

type newSessionSubmit struct {
	Name string
	Dir  string
}

func newSessionFormModel(mode newSessionMode, defaultDir string, width int, hiddenMode CompletionHiddenMode, visibleCount int) (newSessionForm, tea.Cmd) {
	nameInput := textinput.New()
	nameInput.Prompt = "Session name: "
	nameInput.Placeholder = "work"
	nameInput.CharLimit = 64
	nameInput.SetWidth(max(20, min(64, width-18)))

	dirInput := textinput.New()
	dirInput.Prompt = "Start directory: "
	dirInput.Placeholder = defaultDir
	dirInput.SetValue(defaultDir)
	dirInput.SetWidth(max(20, min(80, width-20)))

	completionOptions := directoryCompletionOptions{HiddenMode: hiddenMode, VisibleLimit: visibleCount}
	form := newSessionForm{
		mode:              mode,
		active:            newSessionNameField,
		defaultDir:        defaultDir,
		completionOptions: completionOptions,
		completion:        newDirectoryCompletion(completionOptions),
		name:              nameInput,
		dir:               dirInput,
	}

	return form, form.focusActive()
}

func (f newSessionForm) withDir() bool {
	return f.mode == newSessionWithDir
}

func (f *newSessionForm) setWidth(width int) {
	f.name.SetWidth(max(20, min(64, width-18)))
	f.dir.SetWidth(max(20, min(80, width-20)))
}

func (f newSessionForm) update(msg tea.Msg, keys keyMap) (newSessionForm, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if ok && f.withDir() {
		if f.handleCompletionKey(keyMsg, keys) {
			return f, nil
		}

		switch {
		case key.Matches(keyMsg, keys.NextField):
			f.nextField()
			return f, f.focusActive()
		case key.Matches(keyMsg, keys.PrevField):
			f.prevField()
			return f, f.focusActive()
		case key.Matches(keyMsg, keys.AcceptCompletion):
			return f, nil
		}
	}

	var cmd tea.Cmd
	if f.active == newSessionDirField && f.withDir() {
		oldValue := f.dir.Value()
		f.dir, cmd = f.dir.Update(msg)
		if f.dir.Value() != oldValue {
			f.refreshDirectoryCompletion()
		}
	} else {
		f.name, cmd = f.name.Update(msg)
		f.closeCompletion()
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() != "enter" {
		f.err = ""
	}

	return f, cmd
}

func (f *newSessionForm) nextField() {
	if !f.withDir() {
		return
	}
	if f.active == newSessionNameField {
		f.active = newSessionDirField
	} else {
		f.active = newSessionNameField
	}
	f.closeCompletion()
}

func (f *newSessionForm) prevField() {
	f.nextField()
}

func (f *newSessionForm) focusActive() tea.Cmd {
	f.name.Blur()
	f.dir.Blur()
	if f.active == newSessionDirField && f.withDir() {
		return f.dir.Focus()
	}

	return f.name.Focus()
}

func (f *newSessionForm) closeCompletion() {
	f.completion.setItems(nil)
}

func (f newSessionForm) completionLayerOpen() bool {
	return f.withDir() && f.active == newSessionDirField && f.completion.open()
}

func (f *newSessionForm) handleCompletionKey(keyMsg tea.KeyPressMsg, keys keyMap) bool {
	if !f.completionLayerOpen() {
		return false
	}

	switch {
	case key.Matches(keyMsg, keys.Dismiss):
		f.closeCompletion()
		return true
	case key.Matches(keyMsg, keys.NextCompletion):
		f.completion.next()
		return true
	case key.Matches(keyMsg, keys.PrevCompletion):
		f.completion.prev()
		return true
	case key.Matches(keyMsg, keys.AcceptCompletion):
		if f.acceptCompletion() {
			f.err = ""
		}
		return true
	default:
		return false
	}
}

func (f *newSessionForm) refreshDirectoryCompletion() {
	if !f.withDir() || f.active != newSessionDirField {
		f.closeCompletion()
		return
	}
	if f.completionSuppressedFor != "" {
		if f.dir.Value() == f.completionSuppressedFor {
			f.closeCompletion()
			return
		}
		f.completionSuppressedFor = ""
	}

	f.completion.setItems(directoryCompletionItems(f.dir.Value(), f.defaultDir, f.completionOptions))
}

func (f *newSessionForm) acceptCompletion() bool {
	item, ok := f.completion.selectedItem()
	if !ok {
		return false
	}

	f.dir.SetValue(item.Value)
	f.dir.CursorEnd()
	f.completionSuppressedFor = item.Value
	f.closeCompletion()
	return true
}

func (f *newSessionForm) submit() (newSessionSubmit, error) {
	name, err := herdr.ValidateSessionName(f.name.Value())
	if err != nil {
		f.active = newSessionNameField
		return newSessionSubmit{}, err
	}
	if herdr.IsAttachHelpName(name) {
		f.active = newSessionNameField
		return newSessionSubmit{}, fmt.Errorf("session name %q cannot be attached by Herdr", name)
	}
	if herdr.IsJSONFlagName(name) {
		f.active = newSessionNameField
		return newSessionSubmit{}, fmt.Errorf("session name %q cannot be stopped or deleted by Herdr", name)
	}

	dirInput := f.defaultDir
	if f.withDir() {
		dirInput = f.dir.Value()
	}

	var dir string
	if f.withDir() {
		dir, err = herdr.ResolveOrCreateStartDir(dirInput, f.defaultDir)
	} else {
		dir, err = herdr.ResolveStartDir(dirInput, f.defaultDir)
	}
	if err != nil {
		if f.withDir() {
			f.active = newSessionDirField
		}
		return newSessionSubmit{}, err
	}

	return newSessionSubmit{Name: name, Dir: dir}, nil
}

func (f newSessionForm) render(width int) string {
	title := "New session"
	if f.withDir() {
		title = "New session in directory"
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(textInputView(f.name))
	if f.withDir() {
		b.WriteString("\n")
		b.WriteString(textInputView(f.dir))
		if f.completion.open() {
			b.WriteString("\n")
			b.WriteString(f.completion.render(width))
		}
	}
	if f.err != "" {
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render(sanitizeMultilineDisplay(f.err)))
	}
	b.WriteString("\n\n")
	if f.withDir() {
		b.WriteString(subtleStyle.Render("↑/↓ moves fields or selects matches. Tab completes. Enter creates. Esc closes matches or cancels."))
	} else {
		b.WriteString(subtleStyle.Render("Enter to create and attach. Esc to cancel."))
	}

	return confirmBoxStyle.Width(max(40, min(88, width-4))).Render(b.String())
}

func textInputView(input textinput.Model) string {
	input.SetValue(sanitizeDisplay(input.Value()))
	input.Placeholder = sanitizeDisplay(input.Placeholder)

	return input.View()
}

func (f *newSessionForm) setError(err error) {
	if err == nil {
		f.err = ""
		return
	}

	f.err = fmt.Sprint(err)
}
