package grid

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

func relationFixture() Relation {
	return Relation{
		ID: RelationID{Schema: "public", Name: "users"}, Kind: RelationTable,
		Identity: []string{"id"}, IdentityPrimary: true, CanSelect: true, HasXMin: true,
		CanInsert: true, CanUpdate: true, CanDelete: true,
		Columns: []Column{
			{Name: "id", DataType: "bigint", TypeOID: 20, Sortable: true, CanSelect: true, CanInsert: true, CanUpdate: true, IdentityPart: true},
			{Name: "email", DataType: "text", TypeOID: 25, Sortable: true, CanSelect: true, CanInsert: true, CanUpdate: true},
			{Name: "payload", DataType: "jsonb", TypeOID: 3802, Nullable: true, CanSelect: true, CanInsert: true, CanUpdate: true},
		},
	}
}

func pageFixture() Page {
	return Page{
		Rows: []Row{
			{Identity: map[string]any{"id": int64(1)}, XMin: 11, Cells: []Cell{{Raw: int64(1), Display: "1", Edit: "1"}, {Raw: "a@example.com", Display: "a@example.com", Edit: "a@example.com"}, {Raw: `{}`, Display: `{}`, Edit: `{}`}}},
			{Identity: map[string]any{"id": int64(2)}, XMin: 12, Cells: []Cell{{Raw: int64(2), Display: "2", Edit: "2"}, {Raw: "long-address@example.com", Display: "long-address@example.com", Edit: "long-address@example.com"}, {Null: true, Display: "NULL"}}},
		},
		NextCursor: "next",
	}
}

func loadedModel(t *testing.T) Model {
	t.Helper()
	model := NewModel("workspace-1", "table-1", RelationID{Schema: "public", Name: "users"})
	model.SetSize(64, 14)
	describe := model.Init()().(DescribeIntent)
	pageCommand := model.Update(RelationDescribedMsg{Relation: relationFixture(), Meta: describe.Meta})
	pageIntent := pageCommand().(LoadPageIntent)
	model.Update(PageLoadedMsg{Page: pageFixture(), Meta: pageIntent.Meta})
	return model
}

func TestDescribeThenLoadsFirstHundredRows(t *testing.T) {
	model := NewModel("workspace-1", "table-1", RelationID{Schema: "public", Name: "users"})
	describe := model.Init()().(DescribeIntent)
	if describe.Relation.Name != "users" || describe.Meta.Tab != "table-1" || model.Lifecycle() != LifecycleRunning {
		t.Fatalf("describe intent = %#v lifecycle=%s", describe, model.Lifecycle())
	}
	command := model.Update(RelationDescribedMsg{Relation: relationFixture(), Meta: describe.Meta})
	page := command().(LoadPageIntent)
	if page.Page.Direction != PageFirst || page.Page.Limit != 100 || page.Page.Sort != (Sort{}) {
		t.Fatalf("first page intent = %#v", page)
	}
	model.Update(PageLoadedMsg{Page: pageFixture(), Meta: page.Meta})
	if model.Busy() || model.pageNumber != 1 || len(model.page.Rows) != 2 || model.Lifecycle() != LifecycleIdle {
		t.Fatalf("loaded model = %#v", model)
	}
}

func TestPaginationSortAndRefreshResetPredictably(t *testing.T) {
	model := loadedModel(t)
	command := model.Update(tea.KeyPressMsg{Code: 'n'})
	next := command().(LoadPageIntent)
	if next.Page.Cursor != "next" || next.Page.Direction != PageNext || model.pendingPage != 2 {
		t.Fatalf("next intent = %#v", next)
	}
	model.Update(PageLoadedMsg{Page: Page{Rows: pageFixture().Rows, PrevCursor: "prev"}, Meta: next.Meta})
	if model.pageNumber != 2 {
		t.Fatalf("page number = %d", model.pageNumber)
	}
	command = model.Update(tea.KeyPressMsg{Code: 'p'})
	previous := command().(LoadPageIntent)
	if previous.Page.Direction != PagePrevious || previous.Page.Cursor != "prev" || model.pendingPage != 1 {
		t.Fatalf("previous intent = %#v", previous)
	}
	model.Update(PageLoadedMsg{Page: pageFixture(), Meta: previous.Meta})

	model.selectedColumn = 1
	command = model.Update(tea.KeyPressMsg{Code: 's'})
	sorted := command().(LoadPageIntent)
	if sorted.Page.Sort != (Sort{Column: "email", Ascending: true}) || sorted.Page.Direction != PageFirst || model.pendingPage != 1 {
		t.Fatalf("sort intent = %#v", sorted)
	}
	model.Update(PageLoadedMsg{Page: pageFixture(), Meta: sorted.Meta})
	command = model.Update(tea.KeyPressMsg{Code: 'r'})
	refresh := command().(LoadPageIntent)
	if refresh.Page.Sort != sorted.Page.Sort || refresh.Page.Cursor != "" || refresh.Page.Direction != PageFirst {
		t.Fatalf("refresh intent = %#v", refresh)
	}
}

func TestStalePageAndCancellationCannotOverwriteCurrentState(t *testing.T) {
	model := loadedModel(t)
	command := model.Update(tea.KeyPressMsg{Code: 'r'})
	current := command().(LoadPageIntent)
	stale := current.Meta
	stale.Request++
	stale.Operation = "grid.page.table-1.999"
	model.Update(PageLoadedMsg{Page: Page{Rows: []Row{{Cells: []Cell{{Display: "stale"}}}}}, Meta: stale})
	if len(model.page.Rows) != 2 {
		t.Fatal("stale page replaced current rows")
	}
	cancel := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})().(CancelIntent)
	if cancel.Operation != current.Meta.Operation {
		t.Fatalf("cancel = %#v", cancel)
	}
	model.Update(PageFailed(current.Meta, core.NewError("fetch page", core.ErrorCancellation, "Operation cancelled.", true, nil)))
	if model.Busy() || model.Lifecycle() != LifecycleIdle || model.err != nil || !strings.Contains(model.status, "run it again") {
		t.Fatalf("cancelled model = %#v", model)
	}
}

func TestGridNavigationKeepsSelectedCellVisibleAndMouseEquivalent(t *testing.T) {
	model := loadedModel(t)
	model.SetSize(18, 8)
	model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if model.selectedColumn != 2 || model.columnOffset == 0 {
		t.Fatalf("horizontal state: selected=%d offset=%d", model.selectedColumn, model.columnOffset)
	}
	model.Update(tea.MouseClickMsg{X: 3, Y: 6, Button: tea.MouseLeft})
	if model.selectedRow != 1 {
		t.Fatalf("mouse selected row %d", model.selectedRow)
	}
	model.Update(tea.MouseWheelMsg{X: 3, Y: 5, Button: tea.MouseWheelUp})
	if model.selectedRow != 0 {
		t.Fatalf("wheel selected row %d", model.selectedRow)
	}
}

func TestViewShowsBestEffortEmptyAndPostgreSQLDetails(t *testing.T) {
	model := loadedModel(t)
	model.page = Page{BestEffort: true}
	view := model.View(ui.DefaultTheme())
	joined := strings.Join(view.Lines, "\n")
	if !strings.Contains(joined, "Best-effort") || !strings.Contains(joined, "No rows found") {
		t.Fatalf("empty view = %q", joined)
	}
	model.err = &core.Error{
		Operation: "fetch page", Category: core.ErrorPermission, Summary: "Permission denied.",
		PostgreSQL: &core.PostgreSQLDetails{SQLState: "42501", Detail: "cannot read table", Hint: "grant SELECT"},
	}
	view = model.View(ui.DefaultTheme())
	joined = strings.Join(view.Lines, "\n")
	for _, expected := range []string{"Permission denied", "SQLSTATE 42501", "cannot read table", "grant SELECT"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("error view does not contain %q: %q", expected, joined)
		}
	}
}

func TestViewUsesExactANSIAndUnicodeWidth(t *testing.T) {
	model := loadedModel(t)
	relation := *model.relation
	relation.ID.Name = "利用者🚀"
	model.relationID = relation.ID
	model.relation = &relation
	model.SetSize(48, 12)
	for index, line := range model.View(ui.DefaultTheme()).Lines {
		if width := ansi.StringWidth(line); width != 48 {
			t.Fatalf("line %d width=%d: %q", index, width, line)
		}
	}
}

func TestDraftInsertDistinguishesEmptyTextNullAndDefaultThenAppliesSnapshot(t *testing.T) {
	model := loadedModel(t)
	model.Update(tea.KeyPressMsg{Code: 'i'})
	if len(model.inserts) != 1 || !model.Dirty() || model.selectedRow != 0 {
		t.Fatalf("draft state = %#v", model)
	}
	model.selectedColumn = 0
	model.Update(tea.KeyPressMsg{Code: 'e'})
	model.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.selectedColumn = 1
	model.Update(tea.KeyPressMsg{Code: 'e'})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.selectedColumn = 2
	model.Update(tea.KeyPressMsg{Code: 'z'})
	values := model.inserts[0].Values
	if values["id"].Text != "3" || values["email"].Kind != ValueText || values["email"].Text != "" || values["payload"].Kind != ValueNull {
		t.Fatalf("draft values = %#v", values)
	}
	model.Update(tea.KeyPressMsg{Code: 'a'})
	if model.overlay != overlayApply {
		t.Fatalf("apply overlay = %s status=%q", model.overlay, model.status)
	}
	command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	intent := command().(ApplyIntent)
	if len(intent.Request.Mutations) != 1 || intent.Request.Mutations[0].Values["email"].Kind != ValueText || !model.Busy() {
		t.Fatalf("apply intent = %#v", intent)
	}
	command = model.Update(AppliedMsg{Result: ApplyResult{Inserted: 1}, Meta: intent.Meta})
	if model.Dirty() || command == nil || !model.Busy() {
		t.Fatalf("post-commit reload state = %#v", model)
	}
	page := command().(LoadPageIntent)
	model.Update(PageLoadedMsg{Page: pageFixture(), Meta: page.Meta})
	if !strings.Contains(model.status, "Applied 1 insert") {
		t.Fatalf("post-commit status = %q", model.status)
	}
}

func TestRequiredAndPrimitiveValidationKeepDraftAvailableForCorrection(t *testing.T) {
	model := loadedModel(t)
	model.Update(tea.KeyPressMsg{Code: 'i'})
	model.Update(tea.KeyPressMsg{Code: 'a'})
	if model.overlay != overlayNone || !strings.Contains(model.status, "id is required") || model.selectedColumn != 0 {
		t.Fatalf("required validation state = %#v", model)
	}
	model.clearChanges()
	model.selectedRow = 0
	model.selectedColumn = 0
	model.Update(tea.KeyPressMsg{Code: 'e'})
	model.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.editor == nil || !strings.Contains(model.editor.Error, "integer") || model.Dirty() {
		t.Fatalf("primitive validation state = %#v", model)
	}
}

func TestUpdatesDeletesRowRevertAndDirtyRefreshChoices(t *testing.T) {
	model := loadedModel(t)
	model.selectedColumn = 1
	model.Update(tea.KeyPressMsg{Code: 'e'})
	model.Update(tea.KeyPressMsg{Code: 'x', Text: ".new"})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(model.updates) != 1 || !model.Dirty() {
		t.Fatalf("staged update = %#v", model.updates)
	}
	model.Update(tea.KeyPressMsg{Code: 'd'})
	_, updates, deletes := model.changeCounts()
	if updates != 0 || deletes != 1 {
		t.Fatalf("delete did not supersede update: updates=%d deletes=%d", updates, deletes)
	}
	model.Update(tea.KeyPressMsg{Code: 'd'})
	_, updates, deletes = model.changeCounts()
	if updates != 1 || deletes != 0 {
		t.Fatalf("unstaged delete did not restore update: updates=%d deletes=%d", updates, deletes)
	}
	nextCommand := model.Update(tea.KeyPressMsg{Code: 'n'})
	next := nextCommand().(LoadPageIntent)
	otherPage := pageFixture()
	otherPage.Rows[0].Identity["id"] = int64(3)
	otherPage.Rows[0].Cells[0] = Cell{Raw: int64(3), Display: "3", Edit: "3"}
	otherPage.Rows[1].Identity["id"] = int64(4)
	otherPage.Rows[1].Cells[0] = Cell{Raw: int64(4), Display: "4", Edit: "4"}
	otherPage.NextCursor = ""
	otherPage.PrevCursor = "prev"
	model.Update(PageLoadedMsg{Page: otherPage, Meta: next.Meta})
	if len(model.updates) != 1 || !strings.Contains(strings.Join(model.changeSummaries(), "\n"), "id=1") {
		t.Fatalf("off-page staged update was lost: %#v", model.updates)
	}
	model.Update(tea.KeyPressMsg{Code: 'r'})
	if model.overlay != overlayRefresh || !model.Dirty() {
		t.Fatalf("dirty refresh overlay = %s dirty=%v", model.overlay, model.Dirty())
	}
	command := model.Update(tea.KeyPressMsg{Code: 'r'})
	if model.Dirty() || command == nil {
		t.Fatalf("revert-and-refresh state = %#v", model)
	}
	model.Update(PageLoadedMsg{Page: pageFixture(), Meta: command().(LoadPageIntent).Meta})

	model.selectedColumn = 1
	model.Update(tea.KeyPressMsg{Code: 'e'})
	model.Update(tea.KeyPressMsg{Code: 'x', Text: ".again"})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.Update(tea.KeyPressMsg{Code: 'u'})
	if model.Dirty() {
		t.Fatal("single-row revert left staged changes")
	}
}

func TestApplyFailurePreservesChangesFocusesCellAndShowsPostgreSQLConflict(t *testing.T) {
	model := loadedModel(t)
	model.selectedColumn = 1
	model.Update(tea.KeyPressMsg{Code: 'e'})
	model.Update(tea.KeyPressMsg{Code: 'x', Text: ".changed"})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.Update(tea.KeyPressMsg{Code: 'a'})
	intent := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})().(ApplyIntent)
	structured := core.NewError("update row", core.ErrorConflict, "The row changed after it was loaded.", true, nil)
	structured.PostgreSQL = &core.PostgreSQLDetails{SQLState: "40001", Detail: "concurrent update", Hint: "refresh first"}
	current := pageFixture().Rows[0]
	current.Cells[1] = Cell{Raw: "other", Display: "other", Edit: "other"}
	model.Update(ApplyFailed(intent.Meta, &MutationError{Mutation: 0, Column: "email", Current: &current, Err: structured}))
	if !model.Dirty() || model.overlay != overlayMutationError || model.selectedColumn != 1 || model.Lifecycle() != LifecycleFailed {
		t.Fatalf("failed apply state = %#v", model)
	}
	view := strings.Join(model.View(ui.DefaultTheme()).Lines, "\n")
	for _, expected := range []string{"SQLSTATE 40001", "concurrent update", "refresh first", "Original:", "Staged:", "Current database row:"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("mutation error view missing %q: %q", expected, view)
		}
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !model.Dirty() || model.overlay != overlayNone || model.Lifecycle() != LifecycleIdle {
		t.Fatalf("dismissed apply failure state = %#v", model)
	}
}

func TestGridMutationActionsAreClickable(t *testing.T) {
	model := loadedModel(t)
	model.Update(tea.MouseClickMsg{X: 1, Y: 2, Button: tea.MouseLeft})
	if len(model.inserts) != 1 || !model.Dirty() {
		t.Fatalf("mouse insert state = %#v", model)
	}
	if !strings.Contains(strings.Join(model.View(ui.DefaultTheme()).Lines, "\n"), "!REQUIRED") {
		t.Fatal("draft required marker is not visible")
	}
}

func TestCancelledApplyKeepsCompleteStagedSetRetryable(t *testing.T) {
	model := loadedModel(t)
	model.selectedColumn = 1
	model.Update(tea.KeyPressMsg{Code: 'e'})
	model.Update(tea.KeyPressMsg{Code: 'x', Text: ".changed"})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.Update(tea.KeyPressMsg{Code: 'a'})
	intent := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})().(ApplyIntent)
	stale := intent.Meta
	stale.Request++
	stale.Operation = "grid.apply.table-1.999"
	model.Update(AppliedMsg{Result: ApplyResult{Updated: 1}, Meta: stale})
	if !model.Dirty() || !model.Busy() {
		t.Fatal("stale apply completion cleared the active staged set")
	}
	cancel := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})().(CancelIntent)
	if cancel.Operation != intent.Meta.Operation {
		t.Fatalf("cancel intent = %#v", cancel)
	}
	model.Update(ApplyFailed(intent.Meta, core.NewError("apply changes", core.ErrorCancellation, "Cancelled.", true, nil)))
	if !model.Dirty() || model.Busy() || model.Lifecycle() != LifecycleIdle || !strings.Contains(model.status, "remain") {
		t.Fatalf("cancelled apply state = %#v", model)
	}
}

func TestPrimitiveValidationCoversCommonScalarEditors(t *testing.T) {
	tests := []struct {
		column Column
		value  string
		valid  bool
	}{
		{Column{Name: "enabled", TypeOID: 16}, "yes", true},
		{Column{Name: "enabled", TypeOID: 16}, "perhaps", false},
		{Column{Name: "count", TypeOID: 23}, "-42", true},
		{Column{Name: "count", TypeOID: 23}, "4.2", false},
		{Column{Name: "amount", TypeOID: 1700}, "1.5e6", true},
		{Column{Name: "day", TypeOID: 1082}, "2026-02-30", false},
		{Column{Name: "created", TypeOID: 1184}, "2026-08-19T12:30:00-04:00", true},
		{Column{Name: "identifier", TypeOID: 2950}, "550e8400-e29b-41d4-a716-446655440000", true},
		{Column{Name: "payload", TypeOID: 3802}, "postgres validates this raw text", true},
	}
	for _, test := range tests {
		validation := validatePrimitive(test.column, test.value)
		if (validation == "") != test.valid {
			t.Errorf("validate %s=%q returned %q", test.column.Name, test.value, validation)
		}
	}
}

func TestStagingIsIndependentPerTabAndDirtyCountsStayVisible(t *testing.T) {
	first := loadedModel(t)
	second := loadedModel(t)
	second.tab = "table-2"
	first.Update(tea.KeyPressMsg{Code: 'd'})
	if !first.Dirty() || second.Dirty() {
		t.Fatalf("independent staging: first=%v second=%v", first.Dirty(), second.Dirty())
	}
	view := strings.Join(first.View(ui.DefaultTheme()).Lines, "\n")
	if !strings.Contains(view, "1 staged") || !strings.Contains(view, "1 delete") {
		t.Fatalf("dirty count is not visible: %q", view)
	}
}

func TestApplyReloadsCurrentPageWhileRefreshApplyReturnsToFirstPage(t *testing.T) {
	model := loadedModel(t)
	nextIntent := model.Update(tea.KeyPressMsg{Code: 'n'})().(LoadPageIntent)
	nextPage := pageFixture()
	nextPage.NextCursor = ""
	nextPage.PrevCursor = "prev"
	model.Update(PageLoadedMsg{Page: nextPage, Meta: nextIntent.Meta})
	model.selectedColumn = 1
	model.Update(tea.KeyPressMsg{Code: 'e'})
	model.Update(tea.KeyPressMsg{Code: 'x', Text: ".page-two"})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.Update(tea.KeyPressMsg{Code: 'a'})
	apply := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})().(ApplyIntent)
	reload := model.Update(AppliedMsg{Result: ApplyResult{Updated: 1}, Meta: apply.Meta})().(LoadPageIntent)
	if reload.Page.Direction != PageNext || reload.Page.Cursor != "next" || model.pendingPage != 2 {
		t.Fatalf("normal apply reload = %#v pending page=%d", reload, model.pendingPage)
	}

	model = loadedModel(t)
	model.selectedColumn = 1
	model.Update(tea.KeyPressMsg{Code: 'e'})
	model.Update(tea.KeyPressMsg{Code: 'x', Text: ".refresh"})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.Update(tea.KeyPressMsg{Code: 'r'})
	refreshApply := model.Update(tea.KeyPressMsg{Code: 'a'})().(ApplyIntent)
	reload = model.Update(AppliedMsg{Result: ApplyResult{Updated: 1}, Meta: refreshApply.Meta})().(LoadPageIntent)
	if reload.Page.Direction != PageFirst || reload.Page.Cursor != "" || model.pendingPage != 1 {
		t.Fatalf("refresh apply reload = %#v pending page=%d", reload, model.pendingPage)
	}
}

func TestCellEditorKeepsLongUnicodeCursorVisible(t *testing.T) {
	value := "a very long 利用者 value"
	rendered := editorCursorView(value, len([]rune(value)), 12)
	if !strings.Contains(rendered, "value") || !strings.Contains(rendered, "▏") || ansi.StringWidth(rendered) > 12 {
		t.Fatalf("long editor viewport = %q width=%d", rendered, ansi.StringWidth(rendered))
	}
}

func TestUncertainCommitBlocksBlindRetryAndDirectsReloadRecovery(t *testing.T) {
	model := loadedModel(t)
	model.Update(tea.KeyPressMsg{Code: 'd'})
	model.Update(tea.KeyPressMsg{Code: 'a'})
	intent := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})().(ApplyIntent)
	uncertain := &ApplyUncertainError{Err: core.NewError(
		"commit staged changes", core.ErrorNetwork,
		"The connection was lost while confirming commit. Do not retry until reload.", false, nil,
	)}
	model.Update(ApplyFailed(intent.Meta, uncertain))
	view := strings.Join(model.View(ui.DefaultTheme()).Lines, "\n")
	if !model.Dirty() || !strings.Contains(model.status, "Revert and refresh") || !strings.Contains(view, "Commit confirmation lost") {
		t.Fatalf("uncertain commit recovery state=%#v view=%q", model, view)
	}
}
