package ui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up                key.Binding
	Down              key.Binding
	Attach            key.Binding
	Search            key.Binding
	NewSession        key.Binding
	NewSessionWithDir key.Binding
	Stop              key.Binding
	Delete            key.Binding
	Refresh           key.Binding
	SwitchSearchScope key.Binding
	Confirm           key.Binding
	Cancel            key.Binding
	CloseDialog       key.Binding
	Dismiss           key.Binding
	AcceptCompletion  key.Binding
	NextCompletion    key.Binding
	PrevCompletion    key.Binding
	NextField         key.Binding
	PrevField         key.Binding
	Help              key.Binding
	Quit              key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Attach: key.NewBinding(
			key.WithKeys("enter", "a"),
			key.WithHelp("enter/a", "attach"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		NewSession: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new"),
		),
		NewSessionWithDir: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "new in dir"),
		),
		Stop: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "stop"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		SwitchSearchScope: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "search scope"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("y", "enter"),
			key.WithHelp("y/enter", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("n", "esc"),
			key.WithHelp("n/esc", "cancel"),
		),
		CloseDialog: key.NewBinding(
			key.WithKeys("enter", "esc", "q"),
			key.WithHelp("enter/esc/q", "close"),
		),
		Dismiss: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "dismiss"),
		),
		AcceptCompletion: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "complete"),
		),
		NextCompletion: key.NewBinding(
			key.WithKeys("down", "ctrl+n"),
			key.WithHelp("↓/ctrl+n", "next field/match"),
		),
		PrevCompletion: key.NewBinding(
			key.WithKeys("up", "ctrl+p"),
			key.WithHelp("↑/ctrl+p", "prev field/match"),
		),
		NextField: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "next field"),
		),
		PrevField: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "prev field"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Attach, k.Search, k.NewSession, k.NewSessionWithDir, k.Stop, k.Delete, k.Refresh, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Attach},
		{k.NewSession, k.NewSessionWithDir, k.Stop, k.Delete},
		{k.Search, k.SwitchSearchScope, k.Refresh, k.Help, k.Quit},
	}
}
