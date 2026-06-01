package ui

import (
	"fmt"

	"github.com/j0urneyk/herdrctx/internal/herdr"
)

type confirmAction string

const (
	confirmStop   confirmAction = "stop"
	confirmDelete confirmAction = "delete"
)

type confirmation struct {
	Action  confirmAction
	Session herdr.Session
}

func (c confirmation) title() string {
	switch c.Action {
	case confirmStop:
		return fmt.Sprintf("Stop session %q?", c.Session.Name)
	case confirmDelete:
		return fmt.Sprintf("Delete session %q?", c.Session.Name)
	default:
		return "Confirm action?"
	}
}

func (c confirmation) body() string {
	switch c.Action {
	case confirmStop:
		return "Stopping a session can terminate panes, shells, agents, servers, tests, and other running processes."
	case confirmDelete:
		return "Deleting a session removes its saved session state directory. This cannot be undone."
	default:
		return "This action needs confirmation."
	}
}
