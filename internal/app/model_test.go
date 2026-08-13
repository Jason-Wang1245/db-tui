package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestWindowSizeUpdatesView(t *testing.T) {
	model := New(Dependencies{})
	updated, command := model.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	if command != nil {
		t.Fatal("resize unexpectedly returned a command")
	}

	view := updated.(Model).View()
	if !strings.Contains(view.Content, "100×24") {
		t.Fatalf("view %q does not contain terminal size", view.Content)
	}
}
