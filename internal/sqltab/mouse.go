package sqltab

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

type sqlAction string

const (
	actionExecute sqlAction = "execute"
	actionRunAll  sqlAction = "run-all"
	actionRerun   sqlAction = "rerun"
	actionCancel  sqlAction = "cancel"
)

func (model *Model) updateMouseClick(mouse tea.Mouse) tea.Cmd {
	if mouse.Button != tea.MouseLeft {
		return nil
	}
	if model.inspector {
		return nil
	}
	if mouse.Y == 1 {
		switch actionAt(mouse.X) {
		case actionExecute:
			return model.execute(false)
		case actionRunAll:
			return model.execute(true)
		case actionRerun:
			return model.rerun()
		case actionCancel:
			if model.Busy() {
				model.cancelling = true
				model.status = "Cancelling… PostgreSQL is returning control."
				return messageCommand(CancelIntent{Operation: model.active.Operation})
			}
			model.status = "Nothing is running."
		}
		return nil
	}
	if mouse.Y >= 2 && mouse.Y < 2+model.editorHeight {
		position := model.editorPositionAt(mouse.X, mouse.Y)
		if mouse.Mod&tea.ModShift != 0 {
			if !model.selection.Active {
				model.selection.Anchor = model.cursorOffset()
			}
			model.selection.Head = position
			model.selection.Active = model.selection.Anchor != position
		} else {
			model.selection = selection{Anchor: position, Head: position, Active: false}
		}
		model.setCursorOffset(position)
		model.dragging = true
		model.textMode = true
		model.pane = PaneEditor
		model.editor.Focus()
		model.status = model.executionTargetStatus()
		return nil
	}
	resultDataTop := model.resultTop() + 3
	if mouse.Y >= resultDataTop {
		output, ok := model.currentOutput()
		if !ok || output.Kind != OutputRows {
			return nil
		}
		pageStart, pageEnd := model.pageBounds(output)
		pageStart = max(pageStart, model.resultRowOffset)
		pageEnd = min(pageEnd, pageStart+model.visibleResultRowCount())
		row := pageStart + mouse.Y - resultDataTop
		if row < pageStart || row >= pageEnd {
			return nil
		}
		model.pane = PaneResults
		model.textMode = false
		model.editor.Blur()
		model.selectedRow = row
		model.selectedColumn = model.resultColumnAt(mouse.X, output)
		model.ensureResultRowVisible(output)
	}
	return nil
}

func (model *Model) updateMouseMotion(mouse tea.Mouse) {
	if !model.dragging || mouse.Y < 2 || mouse.Y >= 2+model.editorHeight {
		return
	}
	position := model.editorPositionAt(mouse.X, mouse.Y)
	model.selection.Head = position
	model.selection.Active = model.selection.Anchor != position
	model.setCursorOffset(position)
	model.ensureEditorVisible()
}

func (model *Model) updateMouseWheel(mouse tea.Mouse) {
	if model.inspector {
		if mouse.Button == tea.MouseWheelUp {
			model.inspectorScroll = max(0, model.inspectorScroll-3)
		} else if mouse.Button == tea.MouseWheelDown {
			model.inspectorScroll += 3
		}
		return
	}
	if mouse.Y >= 2 && mouse.Y < 2+model.editorHeight {
		lineCount := len(strings.Split(model.editor.Value(), "\n"))
		if mouse.Button == tea.MouseWheelUp {
			model.editorOffset = max(0, model.editorOffset-3)
		} else if mouse.Button == tea.MouseWheelDown {
			model.editorOffset = min(max(0, lineCount-model.editorHeight), model.editorOffset+3)
		}
		return
	}
	if mouse.Mod&tea.ModShift != 0 {
		if mouse.Button == tea.MouseWheelUp || mouse.Button == tea.MouseWheelLeft {
			model.moveResultColumn(-1)
		} else if mouse.Button == tea.MouseWheelDown || mouse.Button == tea.MouseWheelRight {
			model.moveResultColumn(1)
		}
		return
	}
	if mouse.Button == tea.MouseWheelUp {
		model.moveResultRow(-3)
	} else if mouse.Button == tea.MouseWheelDown {
		model.moveResultRow(3)
	}
}

func actionAt(x int) sqlAction {
	labels := []struct {
		action sqlAction
		width  int
	}{
		{actionExecute, len("[Execute F5]")},
		{actionRunAll, len("[Run all F6]")},
		{actionRerun, len("[Rerun F7]")},
		{actionCancel, len("[Cancel Ctrl+C]")},
	}
	position := 0
	for _, label := range labels {
		if x >= position && x < position+label.width {
			return label.action
		}
		position += label.width + 1
	}
	return ""
}

func (model Model) editorPositionAt(x, y int) int {
	lines := strings.Split(model.editor.Value(), "\n")
	line := min(len(lines)-1, max(0, model.editorOffset+y-2))
	gutterWidth := max(2, len(fmtInt(len(lines)))) + 2
	column := max(0, x-gutterWidth) + model.editorColumnOffset
	runeColumn := runeIndexAtDisplayColumn(lines[line], column)
	offset := 0
	for index := 0; index < line; index++ {
		offset += utf8.RuneCountInString(lines[index]) + 1
	}
	return offset + runeColumn
}

func (model Model) resultColumnAt(x int, output Output) int {
	widths := model.resultColumnWidths(output)
	position := 2
	for _, column := range model.visibleResultColumns(output) {
		if x >= position && x < position+widths[column] {
			return column
		}
		position += widths[column] + 1
	}
	return model.selectedColumn
}

func fmtInt(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
