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
		ReadOnlyReason: "Browsing only.",
		Columns: []Column{
			{Name: "id", DataType: "bigint", TypeOID: 20, Sortable: true, CanSelect: true, IdentityPart: true},
			{Name: "email", DataType: "text", TypeOID: 25, Sortable: true, CanSelect: true},
			{Name: "payload", DataType: "jsonb", TypeOID: 3802, CanSelect: true},
		},
	}
}

func pageFixture() Page {
	return Page{
		Rows: []Row{
			{Identity: map[string]any{"id": int64(1)}, XMin: 11, Cells: []Cell{{Raw: int64(1), Display: "1"}, {Raw: "a@example.com", Display: "a@example.com"}, {Raw: `{}`, Display: `{}`}}},
			{Identity: map[string]any{"id": int64(2)}, XMin: 12, Cells: []Cell{{Raw: int64(2), Display: "2"}, {Raw: "long-address@example.com", Display: "long-address@example.com"}, {Null: true, Display: "NULL"}}},
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
	model.Update(tea.MouseClickMsg{X: 3, Y: 5, Button: tea.MouseLeft})
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
