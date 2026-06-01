package ui

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#8aadf4"))

	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ed8796"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a6da95"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eed49f"))

	completionItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#cad3f5"))

	completionSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#1e1e2e")).
				Background(lipgloss.Color("#8aadf4"))

	dialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			MarginTop(1)

	confirmBoxStyle = dialogBoxStyle.
			BorderForeground(lipgloss.Color("#f5a97f"))
)

func tableStyles() table.Styles {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		BorderBottom(true).
		Bold(true)
	styles.Selected = styles.Selected.
		Foreground(lipgloss.Color("#1e1e2e")).
		Background(lipgloss.Color("#8aadf4")).
		Bold(false)

	return styles
}

func spinnerModel() spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.Line),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#8aadf4"))),
	)
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}

	runes := []rune(value)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}

	return string(runes) + "…"
}
