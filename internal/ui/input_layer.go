package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type inputLayer struct {
	handle func(model, tea.KeyPressMsg) inputLayerResult
}

type inputLayerResult struct {
	model    model
	cmd      tea.Cmd
	consumed bool
}

func dispatchInputKey(m model, msg tea.KeyPressMsg, layers []inputLayer) (tea.Model, tea.Cmd) {
	current := m
	for _, layer := range layers {
		result := layer.handle(current, msg)
		current = result.model
		if result.consumed {
			return current, result.cmd
		}
	}

	return current, nil
}

func (m model) inputLayers() []inputLayer {
	layers := []inputLayer{{handle: handleInterruptInput}}

	if m.dialog != nil {
		return append(layers, inputLayer{handle: handleDialogInput})
	}

	if m.newSession != nil {
		if m.newSession.completionLayerOpen() {
			layers = append(layers, inputLayer{handle: handleNewSessionCompletionInput})
		}
		return append(layers, inputLayer{handle: handleNewSessionFormInput})
	}

	if m.search.active {
		return append(layers, inputLayer{handle: handleSearchInput})
	}

	layers = append(layers, inputLayer{handle: handleRootQuitInput})
	if m.busy != "" {
		return append(layers, inputLayer{handle: handleBusyInput})
	}
	if m.confirm != nil {
		return append(layers, inputLayer{handle: handleConfirmationInput})
	}

	return append(layers, inputLayer{handle: handleRootInput})
}

func handleInterruptInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	if msg.String() == "ctrl+c" {
		return inputConsumed(m, m.quitCmd())
	}

	return inputUnhandled(m)
}

func handleDialogInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	if key.Matches(msg, m.keys.CloseDialog) {
		m.closeDialog()
	}

	return inputConsumed(m, nil)
}

func handleNewSessionCompletionInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	if m.newSession == nil {
		return inputUnhandled(m)
	}

	form := *m.newSession
	if !form.handleCompletionKey(msg, m.keys) {
		return inputUnhandled(m)
	}

	m.newSession = &form
	m.configureTable()
	return inputConsumed(m, nil)
}

func handleNewSessionFormInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	updated, cmd := m.handleNewSessionKey(msg)
	return inputConsumed(updated.(model), cmd)
}

func handleRootQuitInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	if key.Matches(msg, m.keys.Quit) {
		return inputConsumed(m, m.quitCmd())
	}

	return inputUnhandled(m)
}

func handleBusyInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	return inputConsumed(m.handleBusyKey(msg), nil)
}

func handleConfirmationInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	updated, cmd := m.handleConfirmationKey(msg)
	return inputConsumed(updated.(model), cmd)
}

func handleSearchInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	selectedName, oldCursor := m.selectedSessionSnapshot()

	switch {
	case msg.String() == "enter":
		m.search.close()
		m.configureTablePreserving(selectedName, oldCursor)
		return inputConsumed(m, nil)
	case key.Matches(msg, m.keys.Dismiss):
		m.search.clear()
		m.search.close()
		m.configureTablePreserving(selectedName, oldCursor)
		return inputConsumed(m, nil)
	case key.Matches(msg, m.keys.SwitchSearchScope):
		m.search.toggleScope()
		m.configureTablePreserving(selectedName, oldCursor)
		return inputConsumed(m, nil)
	}

	oldQuery := m.search.query()
	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(msg)
	if m.search.query() != oldQuery {
		m.configureTablePreserving(selectedName, oldCursor)
	}

	return inputConsumed(m, cmd)
}

func handleRootInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.table.MoveUp(1)
		m.refreshTableRowsForCurrentCursor()
		return inputConsumed(m, nil)
	case key.Matches(msg, m.keys.Down):
		m.table.MoveDown(1)
		m.refreshTableRowsForCurrentCursor()
		return inputConsumed(m, nil)
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return inputConsumed(m, nil)
	case key.Matches(msg, m.keys.Search):
		selectedName, oldCursor := m.selectedSessionSnapshot()
		cmd := m.search.open(m.width)
		m.configureTablePreserving(selectedName, oldCursor)
		return inputConsumed(m, cmd)
	case key.Matches(msg, m.keys.Refresh):
		if m.loading {
			m.setStatus("Refresh already in progress.", statusInfo)
			return inputConsumed(m, nil)
		}
		cmd := m.startRefresh()
		m.setStatus("Refreshing sessions…", statusInfo)
		return inputConsumed(m, tea.Batch(cmd, m.loadSessionsCmd(), m.spinner.Tick))
	case key.Matches(msg, m.keys.Attach):
		updated, cmd := m.attachSelected()
		return inputConsumed(updated.(model), cmd)
	case key.Matches(msg, m.keys.NewSession):
		updated, cmd := m.openNewSession(newSessionQuick)
		return inputConsumed(updated.(model), cmd)
	case key.Matches(msg, m.keys.NewSessionWithDir):
		updated, cmd := m.openNewSession(newSessionWithDir)
		return inputConsumed(updated.(model), cmd)
	case key.Matches(msg, m.keys.Stop):
		updated, cmd := m.confirmStop()
		return inputConsumed(updated.(model), cmd)
	case key.Matches(msg, m.keys.Delete):
		updated, cmd := m.confirmDelete()
		return inputConsumed(updated.(model), cmd)
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	m.refreshTableRowsForCurrentCursor()
	return inputConsumed(m, cmd)
}

func inputConsumed(m model, cmd tea.Cmd) inputLayerResult {
	return inputLayerResult{model: m, cmd: cmd, consumed: true}
}

func inputUnhandled(m model) inputLayerResult {
	return inputLayerResult{model: m}
}
