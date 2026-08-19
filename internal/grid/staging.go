package grid

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

var (
	numericPattern = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$`)
	uuidPattern    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type cellEditor struct {
	Row    int
	Column int
	Buffer string
	Cursor int
	Error  string
}

type rowReference struct {
	Insert    bool
	InsertIdx int
	PageIdx   int
	Key       string
	Row       Row
}

func (model Model) Captures(message tea.Msg) bool {
	if model.editor == nil && model.overlay == overlayNone {
		return false
	}
	switch message.(type) {
	case tea.KeyPressMsg, tea.PasteMsg, tea.MouseClickMsg, tea.MouseWheelMsg:
		return true
	default:
		return false
	}
}

func (model Model) changeCount() int {
	count := len(model.inserts) + len(model.deletes)
	for key := range model.updates {
		if _, deleted := model.deletes[key]; !deleted {
			count++
		}
	}
	return count
}

func (model Model) changeCounts() (int, int, int) {
	updates := 0
	for key := range model.updates {
		if _, deleted := model.deletes[key]; !deleted {
			updates++
		}
	}
	return len(model.inserts), updates, len(model.deletes)
}

func (model *Model) clearChanges() {
	model.inserts = nil
	model.updates = make(map[string]Mutation)
	model.deletes = make(map[string]Mutation)
	model.editor = nil
	model.overlay = overlayNone
	model.overlayScroll = 0
	model.applySnapshot = nil
	model.applyReloadFirst = false
	model.selectedRow = min(max(0, len(model.page.Rows)-1), max(0, model.selectedRow))
	model.computeColumnWidths()
}

func (model Model) totalRows() int { return len(model.inserts) + len(model.page.Rows) }

func (model Model) rowReferenceAt(index int) (rowReference, bool) {
	if index < 0 || index >= model.totalRows() {
		return rowReference{}, false
	}
	if index < len(model.inserts) {
		mutation := model.inserts[index]
		return rowReference{Insert: true, InsertIdx: index, Row: stagedInsertRow(*model.relation, mutation)}, true
	}
	pageIndex := index - len(model.inserts)
	row := model.page.Rows[pageIndex]
	key := model.identityKey(row.Identity)
	return rowReference{PageIdx: pageIndex, Key: key, Row: row}, true
}

func stagedInsertRow(relation Relation, mutation Mutation) Row {
	row := Row{Cells: make([]Cell, len(relation.Columns))}
	for index, column := range relation.Columns {
		value, ok := mutation.Values[column.Name]
		if !ok {
			row.Cells[index] = Cell{Display: "DEFAULT"}
			continue
		}
		row.Cells[index] = stagedCell(value)
	}
	return row
}

func stagedCell(value StagedValue) Cell {
	switch value.Kind {
	case ValueNull:
		return Cell{Null: true, Display: "NULL"}
	case ValueDefault:
		return Cell{Display: "DEFAULT"}
	default:
		return Cell{Raw: value.Text, Edit: value.Text, Display: safeGridText(value.Text)}
	}
}

func (model Model) cellAt(reference rowReference, column int) Cell {
	if model.relation == nil || column < 0 || column >= len(model.relation.Columns) {
		return Cell{}
	}
	name := model.relation.Columns[column].Name
	if reference.Insert {
		return stagedInsertRow(*model.relation, model.inserts[reference.InsertIdx]).Cells[column]
	}
	if mutation, ok := model.updates[reference.Key]; ok {
		if value, changed := mutation.Values[name]; changed {
			return stagedCell(value)
		}
	}
	if column < len(reference.Row.Cells) {
		return reference.Row.Cells[column]
	}
	return Cell{}
}

func (model *Model) stageInsert() {
	if model.relation == nil || !model.relation.CanInsert {
		model.status = model.insertUnavailableReason()
		return
	}
	model.inserts = append(model.inserts, Mutation{
		Kind: MutationInsert, DraftID: model.nextDraft, Values: make(map[string]StagedValue),
	})
	model.nextDraft++
	model.selectedRow = len(model.inserts) - 1
	model.rowOffset = 0
	model.computeColumnWidths()
	model.status = "Added a pinned draft row. Required values are marked before apply."
}

func (model Model) insertUnavailableReason() string {
	if model.relation == nil {
		return "Table metadata is not available."
	}
	if model.relation.ReadOnlyReason != "" {
		return model.relation.ReadOnlyReason
	}
	if model.relation.InsertReason != "" {
		return model.relation.InsertReason
	}
	return "INSERT is unavailable for this table or role."
}

func (model *Model) openEditor() {
	reference, ok := model.rowReferenceAt(model.selectedRow)
	if !ok || model.relation == nil || model.selectedColumn >= len(model.relation.Columns) {
		return
	}
	if _, deleted := model.deletes[reference.Key]; deleted && !reference.Insert {
		model.status = "Revert the row deletion before editing it."
		return
	}
	column := model.relation.Columns[model.selectedColumn]
	if !model.columnWritable(reference, column) {
		model.status = model.columnReadOnlyReason(reference, column)
		return
	}
	cell := model.cellAt(reference, model.selectedColumn)
	buffer := cell.Edit
	if cell.Null || cell.Display == "DEFAULT" {
		buffer = ""
	}
	model.editor = &cellEditor{
		Row: model.selectedRow, Column: model.selectedColumn,
		Buffer: buffer, Cursor: utf8.RuneCountInString(buffer),
	}
	model.status = "Editing raw value. Enter stages it; Esc cancels. An empty text value remains distinct from NULL."
}

func (model Model) columnReadOnlyReason(reference rowReference, column Column) string {
	if column.Generated || column.Identity {
		return column.Name + " is generated by PostgreSQL and read-only."
	}
	if model.relation.ReadOnlyReason != "" {
		return model.relation.ReadOnlyReason
	}
	if reference.Insert && model.relation.InsertReason != "" {
		return model.relation.InsertReason
	}
	if !reference.Insert && model.relation.UpdateReason != "" {
		return model.relation.UpdateReason
	}
	return column.Name + " is read-only for this operation."
}

func (model Model) columnWritable(reference rowReference, column Column) bool {
	if column.Generated || column.Identity {
		return false
	}
	if reference.Insert {
		return model.relation.CanInsert && column.CanInsert
	}
	return model.relation.CanUpdate && column.CanUpdate
}

func (model *Model) updateEditorPaste(value string) {
	if model.editor == nil {
		return
	}
	model.insertEditorText(value)
}

func (model *Model) updateEditorKey(message tea.KeyPressMsg) tea.Cmd {
	if model.editor == nil {
		return nil
	}
	switch message.Keystroke() {
	case "esc":
		model.editor = nil
		model.status = "Cell edit cancelled."
		return nil
	case "enter":
		model.commitEditor()
		return nil
	case "left":
		model.editor.Cursor = max(0, model.editor.Cursor-1)
		return nil
	case "right":
		model.editor.Cursor = min(utf8.RuneCountInString(model.editor.Buffer), model.editor.Cursor+1)
		return nil
	case "home", "ctrl+a":
		model.editor.Cursor = 0
		return nil
	case "end", "ctrl+e":
		model.editor.Cursor = utf8.RuneCountInString(model.editor.Buffer)
		return nil
	case "backspace":
		model.deleteEditorRune(-1)
		return nil
	case "delete":
		model.deleteEditorRune(1)
		return nil
	}
	if text := message.Key().Text; text != "" && message.Key().Mod&(tea.ModCtrl|tea.ModAlt|tea.ModMeta|tea.ModSuper|tea.ModHyper) == 0 {
		model.insertEditorText(text)
	}
	return nil
}

func (model *Model) insertEditorText(value string) {
	runes := []rune(model.editor.Buffer)
	position := min(len(runes), max(0, model.editor.Cursor))
	runes = append(append(append([]rune(nil), runes[:position]...), []rune(value)...), runes[position:]...)
	model.editor.Buffer = string(runes)
	model.editor.Cursor = position + utf8.RuneCountInString(value)
	model.editor.Error = ""
}

func (model *Model) deleteEditorRune(direction int) {
	runes := []rune(model.editor.Buffer)
	position := min(len(runes), max(0, model.editor.Cursor))
	if direction < 0 && position > 0 {
		runes = append(runes[:position-1], runes[position:]...)
		model.editor.Cursor--
	} else if direction > 0 && position < len(runes) {
		runes = append(runes[:position], runes[position+1:]...)
	}
	model.editor.Buffer = string(runes)
	model.editor.Error = ""
}

func (model *Model) commitEditor() {
	editor := model.editor
	reference, ok := model.rowReferenceAt(editor.Row)
	if !ok || model.relation == nil || editor.Column >= len(model.relation.Columns) {
		model.editor = nil
		return
	}
	column := model.relation.Columns[editor.Column]
	if validation := validatePrimitive(column, editor.Buffer); validation != "" {
		model.editor.Error = validation
		model.status = validation
		return
	}
	model.stageValue(reference, column, StagedValue{Kind: ValueText, Text: editor.Buffer})
	model.editor = nil
	model.computeColumnWidths()
	model.status = "Staged " + column.Name + "."
}

func (model *Model) stageSpecial(kind ValueKind) {
	reference, ok := model.rowReferenceAt(model.selectedRow)
	if !ok || model.relation == nil || model.selectedColumn >= len(model.relation.Columns) {
		return
	}
	if _, deleted := model.deletes[reference.Key]; deleted && !reference.Insert {
		model.status = "Revert the row deletion before changing its values."
		return
	}
	column := model.relation.Columns[model.selectedColumn]
	if !model.columnWritable(reference, column) {
		model.status = model.columnReadOnlyReason(reference, column)
		return
	}
	if kind == ValueNull && !column.Nullable {
		model.status = column.Name + " is NOT NULL."
		return
	}
	if kind == ValueDefault && !column.HasDefault && !column.Nullable {
		model.status = column.Name + " has no default and is NOT NULL."
		return
	}
	model.stageValue(reference, column, StagedValue{Kind: kind})
	model.computeColumnWidths()
	if kind == ValueNull {
		model.status = "Staged SQL NULL for " + column.Name + "."
	} else {
		model.status = "Staged SQL DEFAULT for " + column.Name + "."
	}
}

func (model *Model) stageValue(reference rowReference, column Column, value StagedValue) {
	if reference.Insert {
		mutation := model.inserts[reference.InsertIdx]
		if mutation.Values == nil {
			mutation.Values = make(map[string]StagedValue)
		}
		if value.Kind == ValueDefault {
			delete(mutation.Values, column.Name)
		} else {
			mutation.Values[column.Name] = value
		}
		model.inserts[reference.InsertIdx] = mutation
		return
	}
	mutation, exists := model.updates[reference.Key]
	if !exists {
		mutation = Mutation{Kind: MutationUpdate, Original: cloneRow(reference.Row), Values: make(map[string]StagedValue)}
	}
	if stagedMatchesOriginal(value, reference.Row, model.columnIndex(column.Name)) {
		delete(mutation.Values, column.Name)
	} else {
		mutation.Values[column.Name] = value
	}
	if len(mutation.Values) == 0 {
		delete(model.updates, reference.Key)
	} else {
		model.updates[reference.Key] = mutation
	}
}

func stagedMatchesOriginal(value StagedValue, row Row, column int) bool {
	if column < 0 || column >= len(row.Cells) {
		return false
	}
	cell := row.Cells[column]
	if value.Kind == ValueNull {
		return cell.Null
	}
	return value.Kind == ValueText && !cell.Null && value.Text == cell.Edit
}

func (model *Model) toggleDelete() {
	reference, ok := model.rowReferenceAt(model.selectedRow)
	if !ok || model.relation == nil {
		return
	}
	if reference.Insert {
		model.inserts = append(model.inserts[:reference.InsertIdx], model.inserts[reference.InsertIdx+1:]...)
		model.selectedRow = min(max(0, model.totalRows()-1), model.selectedRow)
		model.computeColumnWidths()
		model.status = "Removed the draft row."
		return
	}
	if !model.relation.CanDelete {
		model.status = model.relation.ReadOnlyReason
		if model.status == "" {
			model.status = model.relation.DeleteReason
		}
		if model.status == "" {
			model.status = "DELETE is unavailable for this table or role."
		}
		return
	}
	if _, staged := model.deletes[reference.Key]; staged {
		delete(model.deletes, reference.Key)
		model.status = "Reverted the staged row deletion."
	} else {
		model.deletes[reference.Key] = Mutation{Kind: MutationDelete, Original: cloneRow(reference.Row)}
		model.status = "Staged row deletion. Apply is required."
	}
}

func (model *Model) revertSelectedRow() {
	reference, ok := model.rowReferenceAt(model.selectedRow)
	if !ok {
		return
	}
	if reference.Insert {
		model.inserts = append(model.inserts[:reference.InsertIdx], model.inserts[reference.InsertIdx+1:]...)
		model.selectedRow = min(max(0, model.totalRows()-1), model.selectedRow)
		model.computeColumnWidths()
		model.status = "Reverted the draft row."
		return
	}
	_, updated := model.updates[reference.Key]
	_, deleted := model.deletes[reference.Key]
	delete(model.updates, reference.Key)
	delete(model.deletes, reference.Key)
	if updated || deleted {
		model.computeColumnWidths()
		model.status = "Reverted all staged changes for the selected row."
	} else {
		model.status = "The selected row has no staged changes."
	}
}

func (model *Model) requestApplyConfirmation() tea.Cmd {
	if !model.Dirty() {
		model.status = "There are no staged changes to apply."
		return nil
	}
	if issue := model.validateChanges(); issue != "" {
		model.status = issue
		return nil
	}
	model.overlay = overlayApply
	model.overlayScroll = 0
	model.applyReloadFirst = false
	model.status = "Review the staged change summary before applying it atomically."
	return nil
}

func (model *Model) validateChanges() string {
	if model.relation == nil {
		return "Table metadata is unavailable."
	}
	for insertIndex, mutation := range model.inserts {
		for columnIndex, column := range model.relation.Columns {
			if column.Nullable || column.HasDefault || column.Generated || column.Identity {
				continue
			}
			value, supplied := mutation.Values[column.Name]
			if !supplied || value.Kind == ValueDefault || value.Kind == ValueNull {
				model.selectedRow = insertIndex
				model.selectedColumn = columnIndex
				model.ensureColumnVisible()
				return column.Name + " is required and has no default."
			}
		}
	}
	return ""
}

func (model *Model) updateOverlayKey(key string) tea.Cmd {
	switch model.overlay {
	case overlayApply:
		switch key {
		case "enter", "a":
			return model.startApply()
		case "esc":
			model.overlay = overlayNone
			model.status = "Apply cancelled; staged changes are unchanged."
		}
		model.updateOverlayScroll(key)
	case overlayRefresh:
		switch key {
		case "a":
			if issue := model.validateChanges(); issue != "" {
				model.overlay = overlayNone
				model.status = issue
				return nil
			}
			model.applyReloadFirst = true
			return model.startApply()
		case "r":
			model.clearChanges()
			return model.startPage(PageRequest{Relation: model.relationID, Direction: PageFirst, Sort: model.sort, Limit: DefaultPageSize}, 1)
		case "esc":
			model.overlay = overlayNone
			model.status = "Refresh cancelled; staged changes are unchanged."
		}
	case overlayRevertAll:
		switch key {
		case "enter", "r":
			model.clearChanges()
			model.status = "Reverted every staged change in this tab."
		case "esc":
			model.overlay = overlayNone
			model.status = "Revert cancelled."
		}
	case overlayChanges:
		switch key {
		case "esc", "v":
			model.overlay = overlayNone
		case "a":
			return model.requestApplyConfirmation()
		default:
			model.updateOverlayScroll(key)
		}
	case overlayMutationError:
		switch key {
		case "esc", "e", "enter":
			model.overlay = overlayNone
			model.err = nil
			model.failedKind = ""
			model.lifecycle = LifecycleIdle
			model.status = "Staged changes are ready for correction or retry."
		case "a":
			model.overlay = overlayNone
			model.err = nil
			model.failedKind = ""
			model.lifecycle = LifecycleIdle
			return model.requestApplyConfirmation()
		}
	}
	return nil
}

func (model *Model) updateOverlayScroll(key string) {
	maximum := max(0, len(model.changeSummaries())-1)
	switch key {
	case "up", "k":
		model.overlayScroll = max(0, model.overlayScroll-1)
	case "down", "j":
		model.overlayScroll = min(maximum, model.overlayScroll+1)
	case "pgup":
		model.overlayScroll = max(0, model.overlayScroll-5)
	case "pgdown":
		model.overlayScroll = min(maximum, model.overlayScroll+5)
	case "home":
		model.overlayScroll = 0
	case "end":
		model.overlayScroll = maximum
	}
}

func (model *Model) startApply() tea.Cmd {
	if model.relation == nil || !model.Dirty() {
		return nil
	}
	snapshot := model.mutationsSnapshot()
	request := model.newRequest()
	meta := model.meta(operationApply, request)
	model.active = activeOperation{Kind: operationApply, Meta: meta}
	model.lifecycle = LifecycleRunning
	model.err = nil
	model.failedKind = ""
	model.overlay = overlayNone
	model.applySnapshot = snapshot
	model.status = "Applying all staged changes in one PostgreSQL transaction…"
	intent := ApplyIntent{Request: ApplyRequest{Relation: *model.relation, Mutations: snapshot}, Meta: meta}
	return func() tea.Msg { return intent }
}

func (model Model) mutationsSnapshot() []Mutation {
	result := make([]Mutation, 0, model.changeCount())
	deleteKeys := sortedMutationKeys(model.deletes)
	for _, key := range deleteKeys {
		result = append(result, cloneMutation(model.deletes[key]))
	}
	updateKeys := sortedMutationKeys(model.updates)
	for _, key := range updateKeys {
		if _, deleted := model.deletes[key]; !deleted {
			result = append(result, cloneMutation(model.updates[key]))
		}
	}
	for _, mutation := range model.inserts {
		result = append(result, cloneMutation(mutation))
	}
	return result
}

func sortedMutationKeys(values map[string]Mutation) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneMutation(mutation Mutation) Mutation {
	copy := mutation
	copy.Original = cloneRow(mutation.Original)
	copy.Values = make(map[string]StagedValue, len(mutation.Values))
	for key, value := range mutation.Values {
		copy.Values[key] = value
	}
	return copy
}

func cloneRow(row Row) Row {
	copy := row
	copy.Identity = make(map[string]any, len(row.Identity))
	for key, value := range row.Identity {
		copy.Identity[key] = value
	}
	copy.Cells = append([]Cell(nil), row.Cells...)
	return copy
}

func (model Model) identityKey(identity map[string]any) string {
	if model.relation == nil {
		return ""
	}
	var builder strings.Builder
	for _, column := range model.relation.Identity {
		value := identity[column]
		encoded := fmt.Sprintf("%T:%#v", value, value)
		fmt.Fprintf(&builder, "%d:%s%d:%s", len(column), column, len(encoded), encoded)
	}
	return builder.String()
}

func (model Model) columnIndex(name string) int {
	if model.relation == nil {
		return -1
	}
	for index, column := range model.relation.Columns {
		if column.Name == name {
			return index
		}
	}
	return -1
}

func (model *Model) focusMutationError(err error) {
	var mutationError *MutationError
	if !errorsAs(err, &mutationError) || mutationError.Mutation < 0 || mutationError.Mutation >= len(model.applySnapshot) {
		return
	}
	mutation := model.applySnapshot[mutationError.Mutation]
	if mutation.Kind == MutationInsert {
		for index := range model.inserts {
			if model.inserts[index].DraftID == mutation.DraftID {
				model.selectedRow = index
				break
			}
		}
	} else {
		key := model.identityKey(mutation.Original.Identity)
		for index, row := range model.page.Rows {
			if model.identityKey(row.Identity) == key {
				model.selectedRow = len(model.inserts) + index
				break
			}
		}
	}
	if column := model.columnIndex(mutationError.Column); column >= 0 {
		model.selectedColumn = column
		model.ensureColumnVisible()
	}
	model.ensureRowVisible()
}

// errorsAs is a small seam kept here to make mutation focus logic easy to test.
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}

func validatePrimitive(column Column, value string) string {
	invalid := func(expected string) string { return column.Name + " expects " + expected + "." }
	switch column.TypeOID {
	case 16:
		switch strings.ToLower(value) {
		case "true", "false", "t", "f", "yes", "no", "on", "off", "1", "0":
			return ""
		default:
			return invalid("a PostgreSQL boolean")
		}
	case 20, 21, 23:
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return invalid("an integer")
		}
	case 26:
		if _, err := strconv.ParseUint(value, 10, 32); err != nil {
			return invalid("an unsigned object identifier")
		}
	case 700, 701:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return invalid("a floating-point number")
		}
	case 1700:
		if value != "NaN" && !numericPattern.MatchString(value) {
			return invalid("a numeric value")
		}
	case 1082:
		if value != "infinity" && value != "-infinity" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return invalid("a date in YYYY-MM-DD form")
			}
		}
	case 1083:
		if !parseAnyTime(value, []string{"15:04", "15:04:05", "15:04:05.999999999"}) {
			return invalid("a time value")
		}
	case 1114:
		if value != "infinity" && value != "-infinity" && !parseAnyTime(value, []string{
			"2006-01-02 15:04", "2006-01-02 15:04:05", "2006-01-02 15:04:05.999999999",
			"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02T15:04:05.999999999",
		}) {
			return invalid("a timestamp")
		}
	case 1184:
		if value != "infinity" && value != "-infinity" && !parseAnyTime(value, []string{
			time.RFC3339, time.RFC3339Nano, "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04:05.999999999Z07:00",
		}) {
			return invalid("a timestamp with time zone")
		}
	case 1266:
		if !parseAnyTime(value, []string{"15:04Z07:00", "15:04:05Z07:00", "15:04:05.999999999Z07:00"}) {
			return invalid("a time with time zone")
		}
	case 2950:
		if !uuidPattern.MatchString(value) {
			return invalid("a UUID")
		}
	}
	return ""
}

func parseAnyTime(value string, layouts []string) bool {
	for _, layout := range layouts {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

func safeGridText(value string) string {
	var builder strings.Builder
	for _, character := range value {
		switch character {
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
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
