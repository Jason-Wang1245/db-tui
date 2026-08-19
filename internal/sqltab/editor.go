package sqltab

import (
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const maxUndoHistory = 200

func (model *Model) updateEditor(message tea.KeyPressMsg) tea.Cmd {
	key := message.Keystroke()
	switch key {
	case "esc":
		model.textMode = false
		model.editor.Blur()
		model.selection.Active = model.hasSelection()
		model.status = "Editor navigation mode. Press e or Enter to edit; Esc returns to the tab strip."
		return nil
	case "ctrl+a":
		model.selection = selection{Anchor: 0, Head: utf8.RuneCountInString(model.editor.Value()), Active: model.editor.Value() != ""}
		model.setCursorOffset(model.selection.Head)
		model.status = model.executionTargetStatus()
		return nil
	case "ctrl+z":
		model.undoEdit()
		return nil
	case "ctrl+y", "ctrl+shift+z":
		model.redoEdit()
		return nil
	case "ctrl+c":
		selected, ok := model.selectedText()
		if !ok {
			model.status = "Select SQL before copying."
			return nil
		}
		model.status = "Copied selected SQL."
		return tea.SetClipboard(selected)
	case "ctrl+x":
		selected, ok := model.selectedText()
		if !ok {
			model.status = "Select SQL before cutting."
			return nil
		}
		model.replaceSelection("")
		model.status = "Cut selected SQL."
		return tea.SetClipboard(selected)
	case "tab":
		model.replaceSelection("  ")
		return nil
	}

	if isSelectionMovement(key) {
		if !model.selection.Active {
			position := model.cursorOffset()
			model.selection = selection{Anchor: position, Head: position, Active: true}
		}
		plain := message.Key()
		plain.Mod &^= tea.ModShift
		plain.Text = ""
		model.editor, _ = model.editor.Update(tea.KeyPressMsg(plain))
		model.selection.Head = model.cursorOffset()
		model.selection.Active = model.selection.Anchor != model.selection.Head
		model.ensureEditorVisible()
		model.status = model.executionTargetStatus()
		return nil
	}

	if model.hasSelection() {
		switch key {
		case "backspace", "delete":
			model.replaceSelection("")
			return nil
		case "enter":
			model.replaceSelection("\n")
			return nil
		}
		if text := message.Key().Text; text != "" && message.Key().Mod&(tea.ModCtrl|tea.ModAlt|tea.ModMeta|tea.ModSuper|tea.ModHyper) == 0 {
			model.replaceSelection(text)
			return nil
		}
	}

	if isPlainMovement(key) {
		model.selection = selection{}
	}
	before := editorSnapshot{Value: model.editor.Value(), Cursor: model.cursorOffset()}
	updated, command := model.editor.Update(message)
	model.editor = updated
	if model.editor.Value() != before.Value {
		model.pushUndo(before)
		model.selection = selection{}
	}
	model.ensureEditorVisible()
	model.status = model.executionTargetStatus()
	return command
}

func isSelectionMovement(key string) bool {
	switch key {
	case "shift+left", "shift+right", "shift+up", "shift+down", "shift+home", "shift+end",
		"ctrl+shift+home", "ctrl+shift+end", "alt+shift+left", "alt+shift+right":
		return true
	default:
		return false
	}
}

func isPlainMovement(key string) bool {
	switch key {
	case "left", "right", "up", "down", "home", "end", "pgup", "pgdown",
		"ctrl+home", "ctrl+end", "alt+left", "alt+right":
		return true
	default:
		return false
	}
}

func (model *Model) replaceSelection(insert string) {
	before := editorSnapshot{Value: model.editor.Value(), Cursor: model.cursorOffset()}
	runes := []rune(before.Value)
	start, end := model.selectionBounds()
	if !model.hasSelection() {
		start = before.Cursor
		end = before.Cursor
	}
	replacement := []rune(insert)
	value := string(append(append(append([]rune(nil), runes[:start]...), replacement...), runes[end:]...))
	model.editor.SetValue(value)
	model.setCursorOffset(start + len(replacement))
	model.selection = selection{}
	if value != before.Value {
		model.pushUndo(before)
	}
	model.ensureEditorVisible()
	model.status = model.executionTargetStatus()
}

func (model *Model) pushUndo(snapshot editorSnapshot) {
	model.undo = append(model.undo, snapshot)
	if len(model.undo) > maxUndoHistory {
		model.undo = append([]editorSnapshot(nil), model.undo[len(model.undo)-maxUndoHistory:]...)
	}
	model.redo = nil
}

func (model *Model) undoEdit() {
	if len(model.undo) == 0 {
		model.status = "Nothing to undo."
		return
	}
	current := editorSnapshot{Value: model.editor.Value(), Cursor: model.cursorOffset()}
	snapshot := model.undo[len(model.undo)-1]
	model.undo = model.undo[:len(model.undo)-1]
	model.redo = append(model.redo, current)
	model.restoreEditor(snapshot)
	model.status = "Undid the last edit."
}

func (model *Model) redoEdit() {
	if len(model.redo) == 0 {
		model.status = "Nothing to redo."
		return
	}
	current := editorSnapshot{Value: model.editor.Value(), Cursor: model.cursorOffset()}
	snapshot := model.redo[len(model.redo)-1]
	model.redo = model.redo[:len(model.redo)-1]
	model.undo = append(model.undo, current)
	model.restoreEditor(snapshot)
	model.status = "Redid the last edit."
}

func (model *Model) restoreEditor(snapshot editorSnapshot) {
	model.editor.SetValue(snapshot.Value)
	model.setCursorOffset(snapshot.Cursor)
	model.selection = selection{}
	model.ensureEditorVisible()
}

func (model Model) cursorOffset() int {
	lines := strings.Split(model.editor.Value(), "\n")
	line := min(model.editor.Line(), len(lines)-1)
	offset := 0
	for index := 0; index < line; index++ {
		offset += utf8.RuneCountInString(lines[index]) + 1
	}
	return offset + min(model.editor.Column(), utf8.RuneCountInString(lines[line]))
}

func (model *Model) setCursorOffset(offset int) {
	value := []rune(model.editor.Value())
	offset = min(len(value), max(0, offset))
	line := 0
	column := 0
	for _, character := range value[:offset] {
		if character == '\n' {
			line++
			column = 0
		} else {
			column++
		}
	}
	model.editor.MoveToBegin()
	for range line {
		model.editor.CursorDown()
	}
	model.editor.SetCursorColumn(column)
}

func (model Model) selectionBounds() (int, int) {
	start, end := model.selection.Anchor, model.selection.Head
	if start > end {
		start, end = end, start
	}
	limit := utf8.RuneCountInString(model.editor.Value())
	return min(limit, max(0, start)), min(limit, max(0, end))
}

func (model Model) hasSelection() bool {
	start, end := model.selectionBounds()
	return model.selection.Active && start != end
}

func (model Model) selectedText() (string, bool) {
	if !model.hasSelection() {
		return "", false
	}
	start, end := model.selectionBounds()
	return string([]rune(model.editor.Value())[start:end]), true
}

func (model *Model) ensureEditorVisible() {
	line := model.editor.Line()
	if line < model.editorOffset {
		model.editorOffset = line
	}
	if line >= model.editorOffset+max(1, model.editorHeight) {
		model.editorOffset = line - max(1, model.editorHeight) + 1
	}
	lineCount := len(strings.Split(model.editor.Value(), "\n"))
	model.editorOffset = min(max(0, lineCount-max(1, model.editorHeight)), max(0, model.editorOffset))

	lines := strings.Split(model.editor.Value(), "\n")
	line = min(max(0, line), len(lines)-1)
	characters := []rune(lines[line])
	column := min(max(0, model.editor.Column()), len(characters))
	cursorColumn := editorDisplayWidth(characters[:column])
	gutterWidth := max(2, len(fmtInt(len(lines)))) + 2
	available := max(1, model.width-gutterWidth)
	if cursorColumn < model.editorColumnOffset {
		model.editorColumnOffset = cursorColumn
	}
	if cursorColumn >= model.editorColumnOffset+available {
		model.editorColumnOffset = cursorColumn - available + 1
	}
	lineWidth := editorDisplayWidth(characters)
	model.editorColumnOffset = min(max(0, lineWidth+1-available), max(0, model.editorColumnOffset))
}

func editorDisplayWidth(characters []rune) int {
	width := 0
	for _, character := range characters {
		width += ansi.StringWidth(safeEditorRune(character))
	}
	return width
}

func (model *Model) copySelection(forceRow bool) tea.Cmd {
	output, ok := model.currentOutput()
	if !ok || output.Kind != OutputRows || model.selectedRow < 0 || model.selectedRow >= len(output.Rows) {
		model.status = "There is no result value to copy."
		return nil
	}
	rows := make([]int, 0, len(model.selectedRows))
	for row := range model.selectedRows {
		if row >= 0 && row < len(output.Rows) {
			rows = append(rows, row)
		}
	}
	sort.Ints(rows)
	if forceRow && len(rows) == 0 {
		rows = append(rows, model.selectedRow)
	}
	if len(rows) == 0 {
		if model.selectedColumn < 0 || model.selectedColumn >= len(output.Rows[model.selectedRow]) {
			return nil
		}
		model.status = "Copied result cell."
		return tea.SetClipboard(output.Rows[model.selectedRow][model.selectedColumn].Full)
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		values := make([]string, len(output.Rows[row]))
		for column, cell := range output.Rows[row] {
			values[column] = cell.Full
		}
		lines = append(lines, strings.Join(values, "\t"))
	}
	model.status = "Copied selected result row(s)."
	return tea.SetClipboard(strings.Join(lines, "\n"))
}

func runeIndexAtDisplayColumn(value string, column int) int {
	if column <= 0 {
		return 0
	}
	width := 0
	for index, character := range []rune(value) {
		characterWidth := ansi.StringWidth(safeEditorRune(character))
		if width+characterWidth > column {
			return index
		}
		width += characterWidth
	}
	return utf8.RuneCountInString(value)
}
