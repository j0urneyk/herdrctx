package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type dialogKind int

const (
	dialogInfo dialogKind = iota
	dialogWarning
	dialogError
)

type alertDialog struct {
	Kind  dialogKind
	Title string
	Body  string
}

func (d alertDialog) render(width int) string {
	dialogWidth := max(44, min(88, width-4))
	bodyWidth := max(20, dialogWidth-6)

	var b strings.Builder
	b.WriteString(d.titleStyle().Render(sanitizeDisplay(d.Title)))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.Wrap(sanitizeMultilineDisplay(d.Body), bodyWidth, ""))
	b.WriteString("\n\n")
	b.WriteString(subtleStyle.Render("Press Enter, Esc, or q to close."))

	return d.boxStyle().Width(dialogWidth).Render(b.String())
}

func renderOverlay(baseView string, overlayView string, width int, height int) string {
	canvasWidth := max(width, max(lipgloss.Width(baseView), lipgloss.Width(overlayView)))
	canvasHeight := max(height, max(lipgloss.Height(baseView), lipgloss.Height(overlayView)))
	baseView = lipgloss.Place(canvasWidth, canvasHeight, lipgloss.Left, lipgloss.Top, baseView)

	overlayX := max(0, (canvasWidth-lipgloss.Width(overlayView))/2)
	overlayY := max(3, (canvasHeight-lipgloss.Height(overlayView))/3)
	if overlayY+lipgloss.Height(overlayView) > canvasHeight {
		overlayY = max(0, canvasHeight-lipgloss.Height(overlayView))
	}

	baseLayer := lipgloss.NewLayer(baseView).Z(0)
	overlayLayer := lipgloss.NewLayer(overlayView).X(overlayX).Y(overlayY).Z(1)

	return lipgloss.NewCompositor(baseLayer, overlayLayer).Render()
}

func (d alertDialog) titleStyle() lipgloss.Style {
	switch d.Kind {
	case dialogError:
		return errorStyle.Bold(true)
	case dialogWarning:
		return warningStyle.Bold(true)
	default:
		return titleStyle
	}
}

func (d alertDialog) boxStyle() lipgloss.Style {
	style := dialogBoxStyle
	switch d.Kind {
	case dialogError:
		return style.BorderForeground(lipgloss.Color("#ed8796"))
	case dialogWarning:
		return style.BorderForeground(lipgloss.Color("#eed49f"))
	default:
		return style.BorderForeground(lipgloss.Color("#8aadf4"))
	}
}
