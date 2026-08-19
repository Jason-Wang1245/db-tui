package sqltab

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

func sqlModelFixture() Model {
	model := NewModel("workspace-1", "sql-1", "SQL 1")
	model.SetSize(80, 24)
	return model
}

func key(code rune, text string, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text, Mod: mod}
}

func rowsResult(count int) RunResult {
	rows := make([][]Cell, count)
	for index := range rows {
		rows[index] = []Cell{
			{Raw: int64(index + 1), Display: fmtInt(index + 1), Full: fmtInt(index + 1)},
			{Raw: "value", Display: "value", Full: "value"},
		}
	}
	return RunResult{Outputs: []Output{{
		Kind:    OutputRows,
		Columns: []Column{{Name: "id", DataType: "bigint"}, {Name: "name", DataType: "text"}},
		Rows:    rows,
	}}}
}

func TestExecuteUsesImmutableSelectionWhileEditorRemainsEditable(t *testing.T) {
	model := sqlModelFixture()
	model.SetValue("select 1;\nselect 2;")
	model.selection = selection{Anchor: 0, Head: 9, Active: true}
	command := model.Update(key(tea.KeyEnter, "", tea.ModCtrl))
	intent := command().(ExecuteIntent)
	if intent.Request.Snapshot != "select 1;" || !model.Busy() || model.Lifecycle() != LifecycleRunning {
		t.Fatalf("execute intent=%#v model=%#v", intent, model)
	}
	model.Update(key('x', "x", 0))
	if model.Buffer() == intent.Request.Snapshot || intent.Request.Snapshot != model.LastRun().Snapshot {
		t.Fatalf("buffer=%q snapshot=%q", model.Buffer(), intent.Request.Snapshot)
	}
}

func TestRunAllRerunCancellationAndStaleMessages(t *testing.T) {
	model := sqlModelFixture()
	model.SetValue("select 1; select 2;")
	model.selection = selection{Anchor: 0, Head: 9, Active: true}
	command := model.Update(key(tea.KeyF6, "", 0))
	intent := command().(ExecuteIntent)
	if intent.Request.Snapshot != model.Buffer() {
		t.Fatalf("run-all snapshot = %q", intent.Request.Snapshot)
	}
	cancel := model.Update(key('c', "", tea.ModCtrl))().(CancelIntent)
	if cancel.Operation != intent.Request.Meta.Operation || !model.Cancelling() {
		t.Fatalf("cancel = %#v", cancel)
	}
	model.Update(RunFailedMsg{
		Partial: rowsResult(2),
		Err:     core.NewError("execute SQL", core.ErrorCancellation, "Operation cancelled.", true, nil),
		Meta:    intent.Request.Meta,
	})
	if model.Busy() || model.Lifecycle() != LifecycleIdle || !model.result.Outputs[0].Incomplete {
		t.Fatalf("cancelled model = %#v", model)
	}
	rerun := model.Update(key(tea.KeyF7, "", 0))().(ExecuteIntent)
	if rerun.Request.Snapshot != intent.Request.Snapshot || rerun.Request.Meta == intent.Request.Meta {
		t.Fatalf("rerun = %#v", rerun)
	}
	model.Update(RunCompletedMsg{Result: rowsResult(1), Meta: intent.Request.Meta})
	if len(model.result.Outputs[0].Rows) != 2 {
		t.Fatal("stale completion overwrote the cancelled result")
	}
}

func TestEditorSelectionUndoRedoAndTabInsertion(t *testing.T) {
	model := sqlModelFixture()
	model.SetValue("abc")
	model.editor.MoveToBegin()
	model.Update(key(tea.KeyRight, "", tea.ModShift))
	model.Update(key(tea.KeyRight, "", tea.ModShift))
	if selected, ok := model.selectedText(); !ok || selected != "ab" {
		t.Fatalf("selection = %q, %v", selected, ok)
	}
	model.Update(key(tea.KeyTab, "", 0))
	if model.Buffer() != "  c" {
		t.Fatalf("tab replacement = %q", model.Buffer())
	}
	model.Update(key('z', "", tea.ModCtrl))
	if model.Buffer() != "abc" {
		t.Fatalf("undo = %q", model.Buffer())
	}
	model.Update(key('y', "", tea.ModCtrl))
	if model.Buffer() != "  c" {
		t.Fatalf("redo = %q", model.Buffer())
	}
}

func TestClearingEditorClearsDirtyStateAndMouseExecuteIsClickable(t *testing.T) {
	model := sqlModelFixture()
	model.SetValue("select 1")
	model.Update(key('a', "", tea.ModCtrl))
	model.Update(key(tea.KeyDelete, "", 0))
	if model.Dirty() {
		t.Fatalf("cleared buffer remained dirty: %q", model.Buffer())
	}
	model.SetValue("select 2")
	command := model.Update(tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseLeft})
	if intent, ok := command().(ExecuteIntent); !ok || intent.Request.Snapshot != "select 2" {
		t.Fatalf("mouse execute intent = %#v", intent)
	}
}

func TestResultNavigationSelectionCopyAndInspector(t *testing.T) {
	model := sqlModelFixture()
	model.SetValue("select * from generate_series(1, 101)")
	intent := model.Update(key(tea.KeyF5, "", 0))().(ExecuteIntent)
	model.Update(RunCompletedMsg{Result: rowsResult(101), Meta: intent.Request.Meta})
	model.Update(key('n', "n", 0))
	if model.resultPage != 1 || model.selectedRow != 100 {
		t.Fatalf("page=%d row=%d", model.resultPage, model.selectedRow)
	}
	model.Update(key('v', "v", 0))
	if command := model.Update(key('c', "c", 0)); command == nil {
		t.Fatal("copy selected row returned no command")
	}
	model.Update(key(tea.KeyEnter, "", 0))
	if !model.inspector || !strings.Contains(strings.Join(model.View(ui.DefaultTheme()).Lines, "\n"), "Value inspector") {
		t.Fatal("result inspector did not open")
	}
}

func TestIndependentTabsCanRunConcurrently(t *testing.T) {
	first := NewModel("workspace-1", "sql-1", "SQL 1")
	second := NewModel("workspace-1", "sql-2", "SQL 2")
	first.SetValue("select 1")
	second.SetValue("select 2")
	firstIntent := first.Update(key(tea.KeyF5, "", 0))().(ExecuteIntent)
	secondIntent := second.Update(key(tea.KeyF5, "", 0))().(ExecuteIntent)
	if !first.Busy() || !second.Busy() || firstIntent.Request.Meta.Operation == secondIntent.Request.Meta.Operation {
		t.Fatalf("independent runs: first=%#v second=%#v", firstIntent, secondIntent)
	}
}

func TestErrorOutputShowsSubmittedPositionAndFailedLifecycle(t *testing.T) {
	model := sqlModelFixture()
	model.SetValue("select 1;\nselect nope;")
	intent := model.Update(key(tea.KeyF5, "", 0))().(ExecuteIntent)
	runError := core.NewError("execute SQL", core.ErrorValidation, "PostgreSQL: column nope does not exist.", false, nil)
	runError.PostgreSQL = &core.PostgreSQLDetails{SQLState: "42703", Position: 18, Hint: "Check the name"}
	model.Update(RunCompletedMsg{Result: RunResult{Outputs: []Output{{Kind: OutputError, Error: runError}}}, Meta: intent.Request.Meta})
	view := strings.Join(model.View(ui.DefaultTheme()).Lines, "\n")
	for _, expected := range []string{"SQLSTATE 42703", "line 2", "Check the name"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view missing %q: %q", expected, view)
		}
	}
	if model.Lifecycle() != LifecycleFailed {
		t.Fatalf("lifecycle = %s", model.Lifecycle())
	}
}

func TestMouseSelectionAndViewUseExactUnicodeWidth(t *testing.T) {
	model := NewModel("workspace-1", "sql-1", "SQL 利用者🚀")
	model.SetSize(48, 16)
	model.SetValue("select '利用者';")
	model.Update(tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft})
	model.Update(tea.MouseMotionMsg{X: 11, Y: 2, Button: tea.MouseLeft})
	model.Update(tea.MouseReleaseMsg{X: 11, Y: 2, Button: tea.MouseLeft})
	if !model.hasSelection() {
		t.Fatal("mouse drag did not select editor text")
	}
	for index, line := range model.View(ui.DefaultTheme()).Lines {
		if width := ansi.StringWidth(line); width != 48 {
			t.Fatalf("line %d width=%d: %q", index, width, line)
		}
	}
}

func TestLongEditorLinesKeepCursorVisibleAndMouseMappingTracksScroll(t *testing.T) {
	model := NewModel("workspace-1", "sql-1", "SQL 1")
	model.SetSize(24, 14)
	model.SetValue("select 'a very long 利用者 value'")
	if model.editorColumnOffset == 0 {
		t.Fatal("long line did not scroll horizontally to the cursor")
	}
	view := model.View(ui.DefaultTheme())
	if !strings.Contains(view.Lines[2], "value") {
		t.Fatalf("cursor tail is not visible: %q", view.Lines[2])
	}
	position := model.editorPositionAt(4, 2)
	if position == 0 {
		t.Fatal("mouse mapping ignored the horizontal editor offset")
	}
}
