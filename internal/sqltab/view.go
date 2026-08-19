package sqltab

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

type ViewData struct {
	Lines  []string
	Hints  string
	Status string
}

func (model Model) View(theme ui.Theme) ViewData {
	width := max(1, model.width)
	height := max(1, model.height)
	lines := make([]string, height)
	if model.inspector {
		model.renderInspector(lines, width, theme)
		return completedView(lines, width, "↑/↓ scroll · c copy · Esc close", model.status)
	}

	target := "full buffer"
	if model.hasSelection() {
		target = "selection"
	}
	mode := "navigation"
	if model.textMode {
		mode = "editing"
	}
	title := fmt.Sprintf("%s · Editor: %s · Execute: %s", model.title, mode, target)
	lines[0] = fit(theme.Title.Render(title), width)
	if height > 1 {
		lines[1] = fit(model.actionLine(), width)
	}
	model.renderEditor(lines, width)

	resultTop := model.resultTop()
	if resultTop < height {
		lines[resultTop] = fit(strings.Repeat("─", width), width)
	}
	if resultTop+1 < height {
		model.renderOutput(lines, width, resultTop+1, theme)
	}
	status := model.status
	if model.stale {
		status += " Previous output is stale while this run is active."
	}
	return completedView(lines, width, model.hints(), status)
}

func (model Model) actionLine() string {
	execute := "[Execute F5]"
	runAll := "[Run all F6]"
	rerun := "[Rerun F7]"
	cancel := "[Cancel Ctrl+C]"
	if model.Busy() {
		execute = " Execute F5 "
		runAll = " Run all F6 "
		rerun = " Rerun F7 "
	} else {
		cancel = " Cancel Ctrl+C "
	}
	return strings.Join([]string{execute, runAll, rerun, cancel}, " ")
}

func (model Model) renderEditor(lines []string, width int) {
	valueLines := strings.Split(model.editor.Value(), "\n")
	if len(valueLines) == 0 {
		valueLines = []string{""}
	}
	gutterWidth := max(2, len(fmt.Sprint(len(valueLines))))
	lineStart := 0
	for index, value := range valueLines {
		if index < model.editorOffset {
			lineStart += utf8.RuneCountInString(value) + 1
			continue
		}
		row := 2 + index - model.editorOffset
		if row >= len(lines) || row >= 2+model.editorHeight {
			break
		}
		prefix := fmt.Sprintf("%*d │", gutterWidth, index+1)
		body := model.renderEditorLine(value, lineStart)
		if value == "" && model.editor.Value() == "" && !model.textMode {
			body = "Write PostgreSQL here…"
		}
		available := max(1, width-ansi.StringWidth(prefix))
		body = ansi.Cut(body, model.editorColumnOffset, model.editorColumnOffset+available)
		lines[row] = fit(prefix+body, width)
		lineStart += utf8.RuneCountInString(value) + 1
	}
}

func (model Model) renderEditorLine(value string, lineStart int) string {
	characters := []rune(value)
	cursor := model.cursorOffset()
	selectionStart, selectionEnd := model.selectionBounds()
	var builder strings.Builder
	selected := false
	for index, character := range characters {
		position := lineStart + index
		isSelected := model.hasSelection() && position >= selectionStart && position < selectionEnd
		isCursor := model.textMode && position == cursor && !isSelected
		if isSelected != selected {
			if isSelected {
				builder.WriteString("\x1b[7m")
			} else {
				builder.WriteString("\x1b[27m")
			}
			selected = isSelected
		}
		text := safeEditorRune(character)
		if isCursor {
			builder.WriteString("\x1b[7m")
			builder.WriteString(text)
			builder.WriteString("\x1b[27m")
		} else {
			builder.WriteString(text)
		}
	}
	if selected {
		builder.WriteString("\x1b[27m")
	}
	if model.textMode && cursor == lineStart+len(characters) {
		builder.WriteString("▏")
	}
	return builder.String()
}

func safeEditorRune(character rune) string {
	switch character {
	case '\t':
		return "  "
	case '\r':
		return `\r`
	}
	if unicode.IsControl(character) {
		return fmt.Sprintf(`\u%04x`, character)
	}
	return string(character)
}

func (model Model) renderOutput(lines []string, width, top int, theme ui.Theme) {
	if len(model.result.Outputs) == 0 {
		label := "Results"
		if model.Busy() && model.stale {
			label += " · Stale"
		}
		lines[top] = fit(theme.Title.Render(label), width)
		if top+1 < len(lines) {
			if model.Busy() {
				lines[top+1] = fit("Waiting for PostgreSQL…", width)
			} else {
				lines[top+1] = fit("No execution output yet.", width)
			}
		}
		return
	}
	output := model.result.Outputs[model.activeOutput]
	label := fmt.Sprintf("Result %d/%d · %s", model.activeOutput+1, len(model.result.Outputs), outputLabel(output))
	if model.stale {
		label += " · Stale"
	}
	if output.Truncated {
		label += " · Truncated"
	}
	if output.Incomplete {
		label += " · Incomplete — cancelled"
	}
	if output.Duration > 0 {
		label += " · " + conciseDuration(output.Duration)
	}
	if output.Kind == OutputRows {
		pages := max(1, (len(output.Rows)+ResultPageSize-1)/ResultPageSize)
		label += fmt.Sprintf(" · Page %d/%d · %d rows", model.resultPage+1, pages, len(output.Rows))
	}
	lines[top] = fit(theme.Title.Render(label), width)
	switch output.Kind {
	case OutputRows:
		model.renderRows(lines, width, top+1, output)
	case OutputCommand:
		model.renderCommand(lines, width, top+1, output)
	case OutputError:
		model.renderError(lines, width, top+1, output.Error)
	}
}

func outputLabel(output Output) string {
	switch output.Kind {
	case OutputRows:
		return "Rows"
	case OutputCommand:
		return "Command"
	case OutputError:
		return "Error"
	default:
		return "Output"
	}
}

func (model Model) renderRows(lines []string, width, top int, output Output) {
	if top >= len(lines) {
		return
	}
	columns := model.visibleResultColumns(output)
	widths := model.resultColumnWidths(output)
	parts := make([]string, 0, len(columns))
	for _, index := range columns {
		parts = append(parts, fit(output.Columns[index].Name, widths[index]))
	}
	lines[top] = fit("  "+strings.Join(parts, "│"), width)
	pageStart, pageEnd := model.pageBounds(output)
	pageStart = max(pageStart, model.resultRowOffset)
	pageEnd = min(pageEnd, pageStart+model.visibleResultRowCount())
	row := top + 1
	for index := pageStart; index < pageEnd && row < len(lines); index++ {
		prefix := "  "
		if index == model.selectedRow {
			prefix = "> "
		}
		if model.selectedRows[index] {
			prefix = "✓ "
		}
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			value := ""
			if column < len(output.Rows[index]) {
				value = output.Rows[index][column].Display
			}
			if index == model.selectedRow && column == model.selectedColumn {
				value = "[" + value + "]"
			}
			values = append(values, fit(value, widths[column]))
		}
		lines[row] = fit(prefix+strings.Join(values, "│"), width)
		row++
	}
	if len(output.Rows) == 0 && row < len(lines) {
		lines[row] = fit("No rows returned.", width)
		row++
	}
	model.renderNotices(lines, width, row)
}

func (model Model) renderCommand(lines []string, width, top int, output Output) {
	if top < len(lines) {
		label := output.CommandTag
		if label == "" {
			label = "Empty query"
		}
		lines[top] = fit("Command tag: "+label, width)
	}
	if top+1 < len(lines) && output.AffectedRows >= 0 {
		lines[top+1] = fit(fmt.Sprintf("Affected rows: %d", output.AffectedRows), width)
	}
	model.renderNotices(lines, width, top+2)
}

func (model Model) renderNotices(lines []string, width, top int) {
	if len(model.result.Notices) == 0 || top >= len(lines) {
		return
	}
	lines[top] = fit(fmt.Sprintf("Notices (%d):", len(model.result.Notices)), width)
	for index, notice := range model.result.Notices {
		row := top + 1 + index
		if row >= len(lines) {
			break
		}
		label := notice.Severity
		if notice.SQLState != "" {
			label += " " + notice.SQLState
		}
		if label != "" {
			label += ": "
		}
		lines[row] = fit(label+notice.Message, width)
	}
}

func (model Model) renderError(lines []string, width, top int, runError *core.Error) {
	if runError == nil {
		if top < len(lines) {
			lines[top] = fit("PostgreSQL stopped the batch.", width)
		}
		return
	}
	row := top
	appendValue := func(value string) {
		if value == "" {
			return
		}
		for _, wrapped := range wrapSafeText(value, width) {
			if row >= len(lines) {
				return
			}
			lines[row] = fit(wrapped, width)
			row++
		}
	}
	appendValue("Error: " + runError.Summary)
	if runError.PostgreSQL == nil {
		return
	}
	details := runError.PostgreSQL
	appendValue(prefixed("SQLSTATE ", details.SQLState))
	if details.Position > 0 {
		line, column := submittedPosition(model.resultSnapshot, details.Position)
		appendValue(fmt.Sprintf("Submitted SQL position %d · line %d, column %d", details.Position, line, column))
	}
	appendValue(prefixed("Detail: ", details.Detail))
	appendValue(prefixed("Hint: ", details.Hint))
	appendValue(prefixed("Constraint: ", details.Constraint))
	appendValue(prefixed("Context: ", details.Context))
}

func submittedPosition(snapshot string, position int32) (int, int) {
	target := max(0, int(position)-1)
	line, column := 1, 1
	for index, character := range []rune(snapshot) {
		if index >= target {
			break
		}
		if character == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return line, column
}

func (model Model) renderInspector(lines []string, width int, theme ui.Theme) {
	output, ok := model.currentOutput()
	if !ok || output.Kind != OutputRows || model.selectedRow >= len(output.Rows) ||
		model.selectedColumn >= len(output.Rows[model.selectedRow]) {
		return
	}
	column := output.Columns[model.selectedColumn]
	cell := output.Rows[model.selectedRow][model.selectedColumn]
	lines[0] = fit(theme.Title.Render(fmt.Sprintf("Value inspector · %s · row %d", column.Name, model.selectedRow+1)), width)
	if len(lines) > 1 {
		typeLabel := column.DataType
		if cell.Null {
			typeLabel += " · NULL"
		}
		lines[1] = fit(typeLabel, width)
	}
	wrapped := wrapSafeText(cell.Full, width)
	for row := 2; row < len(lines); row++ {
		index := model.inspectorScroll + row - 2
		if index < len(wrapped) {
			lines[row] = fit(wrapped[index], width)
		}
	}
}

func wrapSafeText(value string, width int) []string {
	value = safeTerminalText(value, true)
	if value == "" {
		return []string{""}
	}
	var result []string
	for _, logical := range strings.Split(value, "\n") {
		remaining := logical
		for ansi.StringWidth(remaining) > width {
			part := ansi.Truncate(remaining, width, "")
			result = append(result, part)
			remaining = strings.TrimPrefix(remaining, part)
			if part == "" {
				break
			}
		}
		result = append(result, remaining)
	}
	return result
}

func safeTerminalText(value string, preserveNewlines bool) string {
	var builder strings.Builder
	for _, character := range value {
		switch character {
		case '\n':
			if preserveNewlines {
				builder.WriteRune(character)
			} else {
				builder.WriteString(`\n`)
			}
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			if preserveNewlines {
				builder.WriteString("  ")
			} else {
				builder.WriteString(`\t`)
			}
		default:
			if unicode.IsControl(character) {
				fmt.Fprintf(&builder, `\u%04x`, character)
			} else {
				builder.WriteRune(character)
			}
		}
	}
	return builder.String()
}

func (model Model) resultColumnWidths(output Output) []int {
	widths := make([]int, len(output.Columns))
	pageStart, pageEnd := model.pageBounds(output)
	for index, column := range output.Columns {
		widths[index] = min(32, max(4, ansi.StringWidth(column.Name)+2))
		for row := pageStart; row < pageEnd; row++ {
			if index < len(output.Rows[row]) {
				widths[index] = min(32, max(widths[index], ansi.StringWidth(output.Rows[row][index].Display)+2))
			}
		}
	}
	return widths
}

func (model Model) visibleResultColumns(output Output) []int {
	widths := model.resultColumnWidths(output)
	available := max(1, model.width-2)
	used := 0
	var result []int
	for index := model.columnOffset; index < len(output.Columns); index++ {
		width := widths[index]
		if len(result) > 0 {
			width++
		}
		if used+width > available && len(result) > 0 {
			break
		}
		result = append(result, index)
		used += width
	}
	return result
}

func (model *Model) ensureResultColumnVisible(output Output) {
	if model.selectedColumn < model.columnOffset {
		model.columnOffset = model.selectedColumn
	}
	widths := model.resultColumnWidths(output)
	for model.columnOffset < model.selectedColumn {
		used := 0
		for index := model.columnOffset; index <= model.selectedColumn; index++ {
			if index > model.columnOffset {
				used++
			}
			used += widths[index]
		}
		if used <= max(1, model.width-2) {
			break
		}
		model.columnOffset++
	}
}

func (model Model) resultTop() int { return min(model.height, 2+model.editorHeight) }

func (model Model) hints() string {
	if model.Busy() {
		return "Ctrl+C cancel · editor remains editable · [ / ] switch tab"
	}
	if model.textMode {
		return "F5 execute · F6 run all · F7 rerun · Shift+arrows select · Esc navigate"
	}
	if model.pane == PaneResults {
		return "↑/↓ rows · ←/→ columns · n/p page · {/} output · Enter inspect · v select · c copy"
	}
	return "e/Enter edit · ↓ results · F5 execute · F6 run all · Esc tab strip"
}

func conciseDuration(duration time.Duration) string {
	if duration < time.Millisecond {
		return duration.Round(time.Microsecond).String()
	}
	return duration.Round(time.Millisecond).String()
}

func prefixed(prefix, value string) string {
	if value == "" {
		return ""
	}
	return prefix + value
}

func completedView(lines []string, width int, hints, status string) ViewData {
	for index := range lines {
		lines[index] = fit(lines[index], width)
	}
	return ViewData{Lines: lines, Hints: hints, Status: status}
}

func fit(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) > width {
		value = ansi.Truncate(value, width, "…")
	}
	return value + strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
}
