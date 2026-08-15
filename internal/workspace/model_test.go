package workspace

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

func workspaceFixture(t *testing.T) Model {
	t.Helper()
	model := NewModel("workspace-1", "Local", "app", "localhost:5432", "18.1", nil)
	model.SetSize(120, 30)
	command := model.Init()
	intent, ok := command().(LoadSchemasIntent)
	if !ok {
		t.Fatalf("init message = %T", command())
	}
	model.Update(SchemasLoadedMsg{
		Schemas: []Schema{{Name: "audit"}, {Name: "public"}}, Meta: intent.Meta,
	})
	return model
}

func loadPublicRelations(t *testing.T, model *Model) {
	t.Helper()
	model.moveTree(1)
	command := model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	intent, ok := command().(LoadRelationsIntent)
	if !ok {
		t.Fatalf("expand message = %T", command())
	}
	model.Update(RelationsLoadedMsg{
		Schema: "public",
		Relations: []Relation{
			{Schema: "public", Name: "orders", Kind: RelationTable, CanSelect: true},
			{Schema: "public", Name: "users", Kind: RelationTable, CanSelect: true},
		},
		Meta: intent.Meta,
	})
}

func TestCatalogLoadsSchemasThenRelationsLazily(t *testing.T) {
	model := workspaceFixture(t)
	if model.schemas[1].Loaded || model.schemas[1].Loading {
		t.Fatal("relations loaded before schema expansion")
	}
	loadPublicRelations(t, &model)
	if !model.schemas[1].Loaded || !model.schemas[1].Expanded || len(model.schemas[1].Relations) != 2 {
		t.Fatalf("schema state = %#v", model.schemas[1])
	}
}

func TestOpeningTableDeduplicatesAndRestoresContentFocus(t *testing.T) {
	model := workspaceFixture(t)
	loadPublicRelations(t, &model)
	model.moveTree(1)
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(model.tabs) != 1 || model.tabs[0].Table.Relation.Name != "orders" || model.focus != FocusContent {
		t.Fatalf("opened state: tabs=%#v focus=%s", model.tabs, model.focus)
	}
	model.focus = FocusNavigator
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(model.tabs) != 1 || model.focus != FocusContent {
		t.Fatalf("duplicate open created tabs=%d focus=%s", len(model.tabs), model.focus)
	}
}

func TestMixedTabOrderSQLLabelsAndCloseLifecycle(t *testing.T) {
	model := workspaceFixture(t)
	loadPublicRelations(t, &model)
	model.moveTree(1)
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.focus = FocusTabs
	model.Update(tea.KeyPressMsg{Code: 'n'})
	model.focus = FocusTabs
	model.Update(tea.KeyPressMsg{Code: 'n'})
	if got := []string{model.tabs[0].Envelope.Title, model.tabs[1].Envelope.Title, model.tabs[2].Envelope.Title}; strings.Join(got, ",") != "orders,SQL 1,SQL 2" {
		t.Fatalf("tab order = %v", got)
	}
	model.focus = FocusTabs
	model.Update(tea.KeyPressMsg{Code: 'x'})
	if len(model.tabs) != 2 || model.tabs[1].Envelope.Title != "SQL 1" || model.activeTab != model.tabs[1].Envelope.ID {
		t.Fatalf("close state = %#v active=%s", model.tabs, model.activeTab)
	}
}

func TestStaleCatalogCompletionCannotOverwriteNewerRequest(t *testing.T) {
	model := workspaceFixture(t)
	command := model.refreshSchemas()
	current := command().(LoadSchemasIntent)
	model.Update(SchemasLoadedMsg{
		Schemas: []Schema{{Name: "stale"}},
		Meta:    core.RequestMeta{Workspace: model.id, Operation: "workspace.schemas.999", Request: 999},
	})
	if len(model.schemas) != 2 || model.schemas[0].Name == "stale" {
		t.Fatalf("stale schemas were accepted: %#v", model.schemas)
	}
	model.Update(SchemasLoadedMsg{Schemas: []Schema{{Name: "fresh"}}, Meta: current.Meta})
	if len(model.schemas) != 1 || model.schemas[0].Name != "fresh" {
		t.Fatalf("current schemas were not accepted: %#v", model.schemas)
	}
}

func TestFocusCyclesVisiblePanesAndResponsiveModes(t *testing.T) {
	model := workspaceFixture(t)
	if model.Layout() != LayoutWide || model.Focus() != FocusNavigator {
		t.Fatalf("initial layout=%s focus=%s", model.Layout(), model.Focus())
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if model.Focus() != FocusTabs {
		t.Fatalf("focus = %s", model.Focus())
	}
	model.SetSize(80, 20)
	if model.Layout() != LayoutDrawer {
		t.Fatalf("80x20 layout = %s", model.Layout())
	}
	model.SetSize(60, 14)
	if model.Layout() != LayoutSingle {
		t.Fatalf("60x14 layout = %s", model.Layout())
	}
	view := model.View(ui.DefaultTheme())
	if !strings.Contains(view, "? more") || strings.Count(view, "\n") != 13 {
		t.Fatalf("single-pane view = %q", view)
	}
}

func TestMouseUsesSameOpenAndNewSQLActions(t *testing.T) {
	model := workspaceFixture(t)
	loadPublicRelations(t, &model)
	// The first relation is the row below the expanded public schema.
	model.Update(tea.MouseClickMsg{X: 2, Y: 4, Button: tea.MouseLeft})
	if len(model.tabs) != 1 || model.tabs[0].Envelope.Title != "orders" {
		t.Fatalf("mouse did not open relation: %#v", model.tabs)
	}
	model.rebuildHitboxes()
	var sqlBox Hitbox
	for _, hitbox := range model.hitboxes {
		if hitbox.Kind == HitboxNewSQL {
			sqlBox = hitbox
			break
		}
	}
	model.Update(tea.MouseClickMsg{X: sqlBox.Rect.X, Y: sqlBox.Rect.Y, Button: tea.MouseLeft})
	if len(model.tabs) != 2 || model.tabs[1].Envelope.Kind != TabSQL {
		t.Fatalf("mouse did not open SQL tab: %#v", model.tabs)
	}
}

func TestTabStripKeepsActiveTabVisibleAndMouseCloseMatchesKeyboard(t *testing.T) {
	model := workspaceFixture(t)
	model.SetSize(48, 12)
	for range 8 {
		model.focus = FocusTabs
		model.Update(tea.KeyPressMsg{Code: 'n'})
	}
	view := model.View(ui.DefaultTheme())
	if !strings.Contains(view, "SQL 8") || !strings.Contains(view, "‹") {
		t.Fatalf("active tab was not scrolled into view: %q", view)
	}
	var closeBox Hitbox
	for _, hitbox := range model.hitboxes {
		if hitbox.Kind == HitboxCloseTab && hitbox.Index == len(model.tabs)-1 {
			closeBox = hitbox
			break
		}
	}
	model.Update(tea.MouseClickMsg{X: closeBox.Rect.X, Y: closeBox.Rect.Y, Button: tea.MouseLeft})
	if len(model.tabs) != 7 || model.tabs[len(model.tabs)-1].Envelope.Title != "SQL 7" {
		t.Fatalf("mouse close state = %#v", model.tabs)
	}
}

func TestConnectionLossAndReconnectPreserveTabsWithoutReplay(t *testing.T) {
	model := workspaceFixture(t)
	model.focus = FocusTabs
	model.Update(tea.KeyPressMsg{Code: 'n'})
	tabID := model.activeTab

	command := model.Update(model.CheckConnectionNow())
	check := command().(CheckConnectionIntent)
	model.Update(HealthCheckFailed(check.Meta, core.NewError(
		"check connection", core.ErrorNetwork, "PostgreSQL closed the connection.", true, nil,
	)))
	if model.connection != ConnectionLost || model.activeTab != tabID {
		t.Fatalf("lost state=%s active=%s", model.connection, model.activeTab)
	}

	command = model.Update(tea.KeyPressMsg{Code: 'r'})
	reconnect := command().(ReconnectIntent)
	model.Update(ReconnectedMsg{Meta: reconnect.Meta})
	if model.connection != ConnectionConnected || len(model.tabs) != 1 || model.activeTab != tabID {
		t.Fatalf("reconnected state=%s tabs=%#v", model.connection, model.tabs)
	}
	if !strings.Contains(model.status, "no work was replayed") {
		t.Fatalf("status = %q", model.status)
	}
}

func TestPostgreSQLErrorDetailsAreVisibleInWorkspace(t *testing.T) {
	model := workspaceFixture(t)
	detail := &core.Error{
		Operation: "load relations", Category: core.ErrorPermission, Summary: "Permission denied.",
		PostgreSQL: &core.PostgreSQLDetails{SQLState: "42501", Detail: "role cannot read relation", Hint: "grant SELECT"},
	}
	model.err = detail
	view := model.View(ui.DefaultTheme())
	for _, expected := range []string{"Permission denied", "SQLSTATE 42501", "role cannot read relation", "grant SELECT"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view does not contain %q: %q", expected, view)
		}
	}
}

func TestCancelledHealthCheckDoesNotMarkConnectionLost(t *testing.T) {
	model := workspaceFixture(t)
	command := model.Update(model.CheckConnectionNow())
	check := command().(CheckConnectionIntent)
	model.Update(HealthCheckFailed(check.Meta, core.NewError(
		"check connection", core.ErrorCancellation, "Operation cancelled.", true, nil,
	)))
	if model.connection != ConnectionConnected || model.err != nil {
		t.Fatalf("cancelled health check state=%s error=%v", model.connection, model.err)
	}
}

func TestDirtyTabCloseDefaultsToKeepOpenAndRequiresExplicitDiscard(t *testing.T) {
	model := workspaceFixture(t)
	model.focus = FocusTabs
	model.Update(tea.KeyPressMsg{Code: 'n'})
	tabID := model.activeTab
	model.Update(TabStateChangedMsg{Tab: tabID, Dirty: true, Lifecycle: TabIdle})
	model.focus = FocusTabs
	model.Update(tea.KeyPressMsg{Code: 'x'})
	if model.modal.Kind != modalCloseTab || model.modal.Destructive {
		t.Fatalf("close modal = %#v", model.modal)
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(model.tabs) != 1 || model.modal.Kind != modalNone {
		t.Fatalf("safe default closed tab: tabs=%d modal=%#v", len(model.tabs), model.modal)
	}

	model.focus = FocusTabs
	model.Update(tea.KeyPressMsg{Code: 'x'})
	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(model.tabs) != 0 {
		t.Fatalf("explicit discard left %d tabs", len(model.tabs))
	}
}

func TestClosingRunningTabCancelsItsOperation(t *testing.T) {
	model := workspaceFixture(t)
	model.focus = FocusTabs
	model.Update(tea.KeyPressMsg{Code: 'n'})
	tabID := model.activeTab
	request := core.RequestMeta{Workspace: model.id, Tab: tabID, Operation: "sql.run.7", Request: 7}
	model.Update(TabStateChangedMsg{Tab: tabID, Lifecycle: TabRunning, Request: request})
	model.focus = FocusTabs
	model.Update(tea.KeyPressMsg{Code: 'x'})
	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	intent, ok := command().(CancelIntent)
	if !ok || len(intent.Operations) != 1 || intent.Operations[0] != request.Operation {
		t.Fatalf("cancel intent = %#v", intent)
	}
	if len(model.tabs) != 0 {
		t.Fatal("confirmed running tab remained open")
	}
}

func TestDirtyWorkspaceQuitUsesOneSafeSummaryModal(t *testing.T) {
	model := workspaceFixture(t)
	model.focus = FocusTabs
	model.Update(tea.KeyPressMsg{Code: 'n'})
	model.Update(TabStateChangedMsg{Tab: model.activeTab, Dirty: true, Lifecycle: TabIdle})
	model.Update(tea.KeyPressMsg{Code: 'q'})
	if model.modal.Kind != modalQuit || !strings.Contains(model.View(ui.DefaultTheme()), "Dirty or running tabs") {
		t.Fatalf("quit modal = %#v view=%q", model.modal, model.View(ui.DefaultTheme()))
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if model.modal.Kind != modalNone || len(model.tabs) != 1 {
		t.Fatalf("Esc did not keep work: modal=%#v tabs=%d", model.modal, len(model.tabs))
	}
}

func TestViewPreservesANSIAndUnicodeAtExactTerminalWidth(t *testing.T) {
	model := NewModel("workspace-1", "開発環境🚀", "データベース", "localhost:5432", "18.1", nil)
	model.SetSize(48, 12)
	model.Init()
	view := model.View(ui.DefaultTheme())
	for index, line := range strings.Split(view, "\n") {
		if width := ansi.StringWidth(line); width != 48 {
			t.Fatalf("line %d width = %d, want 48: %q", index, width, line)
		}
	}
}
