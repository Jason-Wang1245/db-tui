package grid

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

func (model Model) renderCellEditor(lines []string, width int, theme ui.Theme) {
	if model.editor == nil || model.relation == nil || model.editor.Column >= len(model.relation.Columns) {
		return
	}
	column := model.relation.Columns[model.editor.Column]
	setLine(lines, 1, width, theme.Title.Render("Edit "+column.Name+" · "+column.DataType))
	setLine(lines, 2, width, "Enter stages the raw value; Esc cancels. Use z outside the editor for NULL and f for DEFAULT.")
	setLine(lines, 4, width, "Value:")
	setLine(lines, 5, width, editorCursorView(model.editor.Buffer, model.editor.Cursor, width))
	if model.editor.Error != "" {
		setLine(lines, 7, width, "Validation: "+model.editor.Error)
	}
}

func editorCursorView(value string, cursor, width int) string {
	characters := []rune(value)
	cursor = min(len(characters), max(0, cursor))
	before := safeGridText(string(characters[:cursor]))
	cursorColumn := ansi.StringWidth(before)
	offset := max(0, cursorColumn-max(1, width)+1)
	var rendered string
	if cursor == len(characters) {
		rendered = before + "▏"
	} else {
		current := safeGridText(string(characters[cursor]))
		after := safeGridText(string(characters[cursor+1:]))
		rendered = before + "\x1b[7m" + current + "\x1b[27m" + after
	}
	available := max(1, width)
	visible := ansi.Cut(rendered, offset, offset+available)
	for ansi.StringWidth(visible) > available && offset < cursorColumn {
		offset++
		visible = ansi.Cut(rendered, offset, offset+available)
	}
	return visible
}

func (model Model) renderOverlay(lines []string, width int, theme ui.Theme) {
	inserts, updates, deletes := model.changeCounts()
	switch model.overlay {
	case overlayApply:
		setLine(lines, 1, width, theme.Title.Render("Apply staged changes?"))
		setLine(lines, 2, width, fmt.Sprintf("One transaction · %d insert · %d update · %d delete", inserts, updates, deletes))
		setLine(lines, 3, width, "Any validation, constraint, permission, or conflict failure rolls back the entire batch.")
		setLine(lines, 4, width, "[Apply Enter/a]  [Cancel Esc]")
		model.renderChangeLines(lines, width, 5)
	case overlayRefresh:
		setLine(lines, 1, width, theme.Title.Render("Refresh with staged changes?"))
		setLine(lines, 2, width, fmt.Sprintf("Pending: %d insert · %d update · %d delete", inserts, updates, deletes))
		setLine(lines, 4, width, "[Apply a]  [Revert and refresh r]  [Cancel Esc]")
		setLine(lines, 6, width, "Refresh never silently discards or rebases staged work.")
	case overlayRevertAll:
		setLine(lines, 1, width, theme.Title.Render("Revert every staged change?"))
		setLine(lines, 2, width, fmt.Sprintf("This removes %d insert(s), %d update(s), and %d delete(s) from this tab.", inserts, updates, deletes))
		setLine(lines, 4, width, "[Revert all Enter/r]  [Keep changes Esc]")
	case overlayChanges:
		setLine(lines, 1, width, theme.Title.Render("Staged change summary"))
		setLine(lines, 2, width, fmt.Sprintf("%d insert · %d update · %d delete", inserts, updates, deletes))
		model.renderChangeLines(lines, width, 4)
	case overlayMutationError:
		model.renderMutationError(lines, width, theme)
	}
}

func (model Model) renderChangeLines(lines []string, width, top int) {
	row := top
	summaries := model.changeSummaries()
	start := min(len(summaries), max(0, model.overlayScroll))
	for _, summary := range summaries[start:] {
		if row >= len(lines) {
			break
		}
		setLine(lines, row, width, summary)
		row++
	}
	if row == top {
		setLine(lines, row, width, "No staged changes.")
	} else if start+row-top < len(summaries) && row < len(lines) {
		setLine(lines, row, width, fmt.Sprintf("… %d more change(s); use ↓/PgDown", len(summaries)-(start+row-top)))
	}
}

func (model Model) changeSummaries() []string {
	mutations := model.mutationsSnapshot()
	result := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		switch mutation.Kind {
		case MutationInsert:
			result = append(result, fmt.Sprintf("+ draft %d · %s", mutation.DraftID, stagedValuesSummary(mutation.Values)))
		case MutationUpdate:
			result = append(result, "~ "+identitySummary(model.relation.Identity, mutation.Original.Identity)+" · "+stagedValuesSummary(mutation.Values))
		case MutationDelete:
			result = append(result, "× "+identitySummary(model.relation.Identity, mutation.Original.Identity))
		}
	}
	return result
}

func stagedValuesSummary(values map[string]StagedValue) string {
	if len(values) == 0 {
		return "all columns DEFAULT"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+stagedValueSummary(values[key]))
	}
	return strings.Join(parts, ", ")
}

func stagedValueSummary(value StagedValue) string {
	switch value.Kind {
	case ValueNull:
		return "NULL"
	case ValueDefault:
		return "DEFAULT"
	default:
		return safeGridText(value.Text)
	}
}

func identitySummary(columns []string, identity map[string]any) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, fmt.Sprintf("%s=%v", column, identity[column]))
	}
	return strings.Join(parts, ", ")
}

func (model Model) renderMutationError(lines []string, width int, theme ui.Theme) {
	title := "Apply failed · transaction rolled back"
	var uncertain *ApplyUncertainError
	if errors.As(model.err, &uncertain) {
		title = "Commit confirmation lost · reload before retrying"
	}
	setLine(lines, 1, width, theme.Title.Render(title))
	row := 2
	appendValue := func(value string) {
		for _, wrapped := range wrapGridText(value, width) {
			if row >= len(lines) {
				return
			}
			setLine(lines, row, width, wrapped)
			row++
		}
	}
	structured := asCoreError(model.err)
	if structured != nil {
		appendValue("Error: " + structured.Summary)
		if structured.PostgreSQL != nil {
			details := structured.PostgreSQL
			appendValue(prefixed("SQLSTATE ", details.SQLState))
			appendValue(prefixed("Detail: ", details.Detail))
			appendValue(prefixed("Hint: ", details.Hint))
			appendValue(prefixed("Constraint: ", details.Constraint))
			appendValue(prefixed("Context: ", details.Context))
		}
	} else if model.err != nil {
		appendValue("Error: " + model.err.Error())
	}
	var mutationError *MutationError
	if errors.As(model.err, &mutationError) && mutationError.Mutation >= 0 && mutationError.Mutation < len(model.applySnapshot) {
		mutation := model.applySnapshot[mutationError.Mutation]
		if mutationError.Column != "" {
			appendValue("Affected column: " + mutationError.Column)
		}
		appendValue("Original: " + model.rowValueSummary(mutation.Original))
		if len(mutation.Values) > 0 {
			appendValue("Staged: " + stagedValuesSummary(mutation.Values))
		}
		if mutationError.Current != nil {
			appendValue("Current database row: " + model.rowValueSummary(*mutationError.Current))
		} else if structured != nil && structured.Category == core.ErrorConflict {
			appendValue("Current database row: no row remains at the original identity.")
		}
	}
}

func (model Model) rowValueSummary(row Row) string {
	if model.relation == nil {
		return "unavailable"
	}
	parts := make([]string, 0, len(model.relation.Columns))
	for index, column := range model.relation.Columns {
		if index < len(row.Cells) {
			parts = append(parts, column.Name+"="+row.Cells[index].Display)
		}
	}
	return strings.Join(parts, ", ")
}

func (model Model) overlayHints() string {
	switch model.overlay {
	case overlayApply:
		return "↑/↓ summary · Enter/a apply atomically · Esc cancel"
	case overlayRefresh:
		return "a apply · r revert and refresh · Esc cancel"
	case overlayRevertAll:
		return "Enter/r revert all · Esc keep changes"
	case overlayChanges:
		return "↑/↓ scroll · a apply · Esc close summary"
	case overlayMutationError:
		return "Esc/e correct staged values · a retry apply"
	default:
		return "Esc close"
	}
}

func wrapGridText(value string, width int) []string {
	value = safeGridText(value)
	if value == "" {
		return nil
	}
	result := make([]string, 0, 1)
	for ansi.StringWidth(value) > width {
		part := ansi.Cut(value, 0, width)
		if part == "" {
			break
		}
		result = append(result, part)
		value = ansi.Cut(value, width, ansi.StringWidth(value))
	}
	if value != "" {
		result = append(result, value)
	}
	return result
}

func setLine(lines []string, row, width int, value string) {
	if row >= 0 && row < len(lines) {
		lines[row] = fit(value, width)
	}
}
