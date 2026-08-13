// Package app owns the root Bubble Tea model and application lifecycle.
package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

type Dependencies struct {
	Diagnostics *Diagnostics
}

type Model struct {
	width       int
	height      int
	diagnostics *Diagnostics
	theme       ui.Theme
}

func New(deps Dependencies) Model {
	diagnostics := deps.Diagnostics
	if diagnostics == nil {
		diagnostics = NewDiagnostics(256)
	}
	return Model{
		diagnostics: diagnostics,
		theme:       ui.DefaultTheme(),
	}
}

func (Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
	case tea.KeyPressMsg:
		switch message.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	lines := []string{
		m.theme.Title.Render("db-tui"),
		"",
		"PostgreSQL connection management is the next implementation slice.",
		"",
		m.theme.Muted.Render("q quit • ? help"),
	}
	if m.width > 0 && m.height > 0 {
		lines = append(lines, "", m.theme.Muted.Render(fmt.Sprintf("%d×%d", m.width, m.height)))
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.ReportFocus = true
	return view
}
