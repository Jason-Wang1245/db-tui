package grid

import (
	"errors"
	"fmt"
	"strings"

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
	title := model.relationID.Schema + "." + model.relationID.Name
	lines[0] = fit(theme.Title.Render(title), width)

	if model.err != nil {
		model.renderError(lines, width)
		return completedView(lines, width, "r retry · Ctrl+C cancel · ? help", model.status)
	}
	if model.relation == nil {
		if height > 2 {
			lines[2] = fit("Loading relation metadata…", width)
		}
		return completedView(lines, width, "Ctrl+C cancel · ? help", model.status)
	}

	notice := model.relation.ReadOnlyReason
	if model.page.BestEffort || model.relation.BestEffort {
		notice = "Best-effort read-only paging: concurrent changes may repeat or skip rows."
	}
	if notice == "" {
		notice = "Read-only browsing · staged changes: 0"
	}
	if height > 1 {
		lines[1] = fit(theme.Muted.Render(notice), width)
	}
	if height < 5 {
		return completedView(lines, width, model.hints(), model.status)
	}

	visible := model.visibleColumns()
	if len(visible) == 0 {
		lines[2] = fit("No readable columns.", width)
		return completedView(lines, width, model.hints(), model.status)
	}
	lines[2] = model.renderHeader(visible, width)
	lines[3] = fit(strings.Repeat("─", width), width)
	rowCapacity := model.visibleRowCount()
	for row := 0; row < rowCapacity && model.rowOffset+row < len(model.page.Rows); row++ {
		index := model.rowOffset + row
		lines[4+row] = model.renderRow(index, visible, width)
	}
	if len(model.page.Rows) == 0 && !model.Busy() {
		lines[4] = fit("No rows found.", width)
	}
	footer := fmt.Sprintf("Page %d · %d rows", max(1, model.pageNumber), len(model.page.Rows))
	if model.page.PrevCursor != "" {
		footer += " · previous"
	}
	if model.page.NextCursor != "" {
		footer += " · next"
	}
	lines[height-1] = fit(theme.Muted.Render(footer), width)
	return completedView(lines, width, model.hints(), model.status)
}

func completedView(lines []string, width int, hints, status string) ViewData {
	for index := range lines {
		lines[index] = fit(lines[index], width)
	}
	return ViewData{Lines: lines, Hints: hints, Status: status}
}

func (model Model) visibleColumns() []int {
	if model.relation == nil || len(model.columnWidths) != len(model.relation.Columns) {
		return nil
	}
	available := max(1, model.width-2)
	used := 0
	indices := make([]int, 0, len(model.relation.Columns)-model.columnOffset)
	for index := model.columnOffset; index < len(model.relation.Columns); index++ {
		width := model.columnWidths[index]
		if len(indices) > 0 {
			width++
		}
		if used+width > available && len(indices) > 0 {
			break
		}
		indices = append(indices, index)
		used += width
	}
	return indices
}

func (model Model) renderHeader(columns []int, width int) string {
	parts := make([]string, 0, len(columns))
	for _, index := range columns {
		column := model.relation.Columns[index]
		label := column.Name
		if column.IdentityPart {
			label += " [key]"
		}
		if model.sort.Column == column.Name {
			if model.sort.Ascending {
				label += " ↑"
			} else {
				label += " ↓"
			}
		}
		parts = append(parts, fit(label, model.columnWidths[index]))
	}
	return fit("  "+strings.Join(parts, "│"), width)
}

func (model Model) renderRow(rowIndex int, columns []int, width int) string {
	row := model.page.Rows[rowIndex]
	prefix := "  "
	if rowIndex == model.selectedRow {
		prefix = "> "
	}
	parts := make([]string, 0, len(columns))
	for _, index := range columns {
		value := ""
		if index < len(row.Cells) {
			value = row.Cells[index].Display
		}
		if rowIndex == model.selectedRow && index == model.selectedColumn {
			value = "[" + value + "]"
		}
		parts = append(parts, fit(value, model.columnWidths[index]))
	}
	return fit(prefix+strings.Join(parts, "│"), width)
}

func (model Model) renderError(lines []string, width int) {
	structured := asCoreError(model.err)
	if structured == nil {
		if len(lines) > 2 {
			lines[2] = fit("Error: "+model.err.Error(), width)
		}
		return
	}
	row := 2
	if row < len(lines) {
		lines[row] = fit("Error: "+structured.Summary, width)
		row++
	}
	if structured.PostgreSQL == nil {
		return
	}
	details := structured.PostgreSQL
	for _, value := range []string{
		prefixed("SQLSTATE ", details.SQLState),
		prefixed("Detail: ", details.Detail),
		prefixed("Hint: ", details.Hint),
		prefixed("Constraint: ", details.Constraint),
		prefixed("Context: ", details.Context),
	} {
		if value != "" && row < len(lines) {
			lines[row] = fit(value, width)
			row++
		}
	}
}

func (model Model) hints() string {
	if model.Busy() {
		return "Ctrl+C cancel · ? help"
	}
	return "↑/↓ rows · ←/→ columns · n/p page · s sort · r refresh"
}

func prefixed(prefix, value string) string {
	if value == "" {
		return ""
	}
	return prefix + value
}

func asCoreError(err error) *core.Error {
	var result *core.Error
	if errors.As(err, &result) {
		return result
	}
	return nil
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

func displayWidth(value string) int { return ansi.StringWidth(value) }
