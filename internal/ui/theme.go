package ui

import "charm.land/lipgloss/v2"

type Theme struct {
	Title lipgloss.Style
	Muted lipgloss.Style
}

func DefaultTheme() Theme {
	return Theme{
		Title: lipgloss.NewStyle().Bold(true),
		Muted: lipgloss.NewStyle().Faint(true),
	}
}
