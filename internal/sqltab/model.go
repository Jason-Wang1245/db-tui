package sqltab

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

type Lifecycle string

const (
	LifecycleIdle    Lifecycle = "idle"
	LifecycleRunning Lifecycle = "running"
	LifecycleFailed  Lifecycle = "failed"
)

type Pane string

const (
	PaneEditor  Pane = "editor"
	PaneResults Pane = "results"
)

type ExecuteIntent struct {
	Request RunRequest
}

type CancelIntent struct {
	Operation core.OperationID
}

type LeaveContentIntent struct {
	Tab core.TabID
}

type RunCompletedMsg struct {
	Result RunResult
	Meta   core.RequestMeta
}

type RunFailedMsg struct {
	Partial RunResult
	Err     error
	Meta    core.RequestMeta
}

type editorSnapshot struct {
	Value  string
	Cursor int
}

type selection struct {
	Anchor int
	Head   int
	Active bool
}

type Model struct {
	workspace core.WorkspaceID
	tab       core.TabID
	title     string

	editor             textarea.Model
	textMode           bool
	pane               Pane
	selection          selection
	undo               []editorSnapshot
	redo               []editorSnapshot
	editorOffset       int
	editorColumnOffset int
	dragging           bool

	width        int
	height       int
	editorHeight int

	lastRun         RunRequest
	resultSnapshot  string
	result          RunResult
	stale           bool
	activeOutput    int
	resultPage      int
	selectedRow     int
	resultRowOffset int
	selectedColumn  int
	columnOffset    int
	selectedRows    map[int]bool
	inspector       bool
	inspectorScroll int

	nextRequest core.RequestID
	active      core.RequestMeta
	lifecycle   Lifecycle
	cancelling  bool
	status      string
}

func NewModel(workspace core.WorkspaceID, tab core.TabID, title string) Model {
	editor := textarea.New()
	editor.Placeholder = "Write PostgreSQL here…"
	editor.ShowLineNumbers = false
	editor.Prompt = ""
	editor.CharLimit = 0
	editor.MaxHeight = 0
	editor.MaxWidth = 0
	editor.MaxContentHeight = 0
	editor.SetVirtualCursor(true)
	editor.Focus()
	return Model{
		workspace:    workspace,
		tab:          tab,
		title:        title,
		editor:       editor,
		textMode:     true,
		pane:         PaneEditor,
		nextRequest:  1,
		lifecycle:    LifecycleIdle,
		selectedRows: make(map[int]bool),
		status:       "Editing SQL. Execute uses the full buffer.",
	}
}

func (model *Model) Init() tea.Cmd { return model.editor.Focus() }

func (model *Model) SetSize(width, height int) {
	model.width = max(1, width)
	model.height = max(1, height)
	if height < 10 {
		model.editorHeight = max(2, height-5)
	} else {
		model.editorHeight = max(4, min(10, height/2))
	}
	model.editor.SetWidth(max(1, width-5))
	model.editor.SetHeight(model.editorHeight)
	model.ensureEditorVisible()
	model.clampResultSelection()
}

func (model Model) Tab() core.TabID              { return model.tab }
func (model Model) Buffer() string               { return model.editor.Value() }
func (model Model) Dirty() bool                  { return model.editor.Value() != "" }
func (model Model) Busy() bool                   { return model.active.Operation != "" }
func (model Model) Cancelling() bool             { return model.cancelling }
func (model Model) Lifecycle() Lifecycle         { return model.lifecycle }
func (model Model) ActiveMeta() core.RequestMeta { return model.active }
func (model Model) Result() RunResult            { return model.result }
func (model Model) LastRun() RunRequest          { return model.lastRun }
func (model Model) Selection() (int, int, bool) {
	start, end := model.selectionBounds()
	return start, end, model.hasSelection()
}
func (model Model) Editing() bool                 { return model.textMode }
func (model Model) Dragging() bool                { return model.dragging }
func (model Model) ActivePane() Pane              { return model.pane }
func (model Model) CurrentOutput() (Output, bool) { return model.currentOutput() }

func (model *Model) SetValue(value string) {
	model.editor.SetValue(value)
	model.editor.MoveToEnd()
	model.selection = selection{}
	model.ensureEditorVisible()
}

func (model *Model) Visit() {
	if model.lifecycle == LifecycleFailed {
		model.lifecycle = LifecycleIdle
	}
}

func (model Model) Captures(message tea.Msg) bool {
	if model.inspector {
		_, key := message.(tea.KeyPressMsg)
		_, click := message.(tea.MouseClickMsg)
		_, wheel := message.(tea.MouseWheelMsg)
		return key || click || wheel
	}
	if model.textMode {
		switch message.(type) {
		case tea.KeyPressMsg, tea.PasteMsg, tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg:
			return true
		}
	}
	switch message := message.(type) {
	case tea.KeyPressMsg:
		switch message.Keystroke() {
		case "f5", "f6", "f7", "ctrl+enter", "ctrl+shift+enter", "ctrl+c",
			"e", "enter", "up", "down", "left", "right", "h", "j", "k", "l",
			"n", "p", "pgdown", "pgup", "{", "}", "v", "c", "shift+c", "home", "end":
			return true
		}
	case tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg:
		return true
	}
	return false
}

func (model *Model) Update(message tea.Msg) tea.Cmd {
	switch message := message.(type) {
	case RunCompletedMsg:
		if !model.active.Matches(message.Meta) {
			return nil
		}
		model.finishRun(message.Result)
		return nil
	case RunFailedMsg:
		if !model.active.Matches(message.Meta) {
			return nil
		}
		return model.finishFailedRun(message)
	case tea.PasteMsg:
		model.Visit()
		if model.textMode {
			model.replaceSelection(message.Content)
		}
	case tea.KeyPressMsg:
		model.Visit()
		return model.updateKey(message)
	case tea.MouseClickMsg:
		model.Visit()
		return model.updateMouseClick(message.Mouse())
	case tea.MouseMotionMsg:
		model.Visit()
		model.updateMouseMotion(message.Mouse())
	case tea.MouseReleaseMsg:
		model.Visit()
		model.dragging = false
	case tea.MouseWheelMsg:
		model.Visit()
		model.updateMouseWheel(message.Mouse())
	}
	return nil
}

func (model *Model) updateKey(message tea.KeyPressMsg) tea.Cmd {
	key := message.Keystroke()
	if model.inspector {
		return model.updateInspectorKey(key)
	}
	if model.Busy() && key == "ctrl+c" {
		model.cancelling = true
		model.status = "Cancelling… PostgreSQL is returning control."
		return messageCommand(CancelIntent{Operation: model.active.Operation})
	}
	switch key {
	case "ctrl+enter", "f5":
		return model.execute(false)
	case "ctrl+shift+enter", "f6":
		return model.execute(true)
	case "f7":
		return model.rerun()
	}
	if model.textMode {
		return model.updateEditor(message)
	}
	switch key {
	case "esc":
		return messageCommand(LeaveContentIntent{Tab: model.tab})
	case "e":
		model.pane = PaneEditor
		model.enterTextMode()
	case "enter":
		if model.pane == PaneEditor {
			model.enterTextMode()
		} else {
			model.openInspector()
		}
	case "up", "k":
		if model.pane == PaneResults {
			model.moveResultRow(-1)
		} else {
			model.pane = PaneEditor
		}
	case "down", "j":
		if len(model.result.Outputs) > 0 {
			model.pane = PaneResults
			model.moveResultRow(1)
		}
	case "left", "h":
		if model.pane == PaneResults {
			model.moveResultColumn(-1)
		}
	case "right", "l":
		if model.pane == PaneResults {
			model.moveResultColumn(1)
		}
	case "n", "pgdown":
		model.moveResultPage(1)
	case "p", "pgup":
		model.moveResultPage(-1)
	case "{":
		model.moveOutput(-1)
	case "}":
		model.moveOutput(1)
	case "home":
		model.selectedRow = 0
		model.resultPage = 0
		model.resultRowOffset = 0
	case "end":
		if output, ok := model.currentOutput(); ok && output.Kind == OutputRows && len(output.Rows) > 0 {
			model.selectedRow = len(output.Rows) - 1
			model.resultPage = model.selectedRow / ResultPageSize
			model.ensureResultRowVisible(output)
		}
	case "v":
		model.toggleSelectedRow()
	case "c":
		return model.copySelection(false)
	case "shift+c":
		return model.copySelection(true)
	case "ctrl+c":
		model.status = "Nothing is running."
	}
	return nil
}

func (model *Model) execute(all bool) tea.Cmd {
	if model.Busy() {
		model.status = "This tab already has a run in progress."
		return nil
	}
	snapshot := model.editor.Value()
	if !all {
		if selected, ok := model.selectedText(); ok {
			snapshot = selected
		}
	}
	if strings.TrimSpace(snapshot) == "" {
		model.status = "Enter SQL before executing."
		return nil
	}
	return model.startRun(snapshot)
}

func (model *Model) rerun() tea.Cmd {
	if model.Busy() {
		model.status = "This tab already has a run in progress."
		return nil
	}
	if model.lastRun.Snapshot == "" {
		model.status = "There is no previous run to rerun."
		return nil
	}
	return model.startRun(model.lastRun.Snapshot)
}

func (model *Model) startRun(snapshot string) tea.Cmd {
	request := model.nextRequest
	model.nextRequest++
	meta := core.RequestMeta{
		Workspace: model.workspace,
		Tab:       model.tab,
		Operation: core.OperationID(fmt.Sprintf("sql.run.%s.%d", model.tab, request)),
		Request:   request,
	}
	run := RunRequest{Meta: meta, Snapshot: snapshot}
	model.lastRun = run
	model.active = meta
	model.lifecycle = LifecycleRunning
	model.cancelling = false
	model.stale = len(model.result.Outputs) > 0
	model.status = "Running immutable SQL snapshot…"
	return messageCommand(ExecuteIntent{Request: run})
}

func (model *Model) finishRun(result RunResult) {
	model.active = core.RequestMeta{}
	model.cancelling = false
	model.stale = false
	model.result = result
	model.resultSnapshot = model.lastRun.Snapshot
	model.resetResultNavigation()
	model.pane = PaneResults
	model.textMode = false
	model.editor.Blur()
	model.lifecycle = LifecycleIdle
	for _, output := range result.Outputs {
		if output.Kind == OutputError {
			model.lifecycle = LifecycleFailed
			break
		}
	}
	if model.lifecycle == LifecycleFailed {
		model.status = "PostgreSQL stopped the batch on an error. Earlier outputs were preserved."
	} else {
		model.status = fmt.Sprintf("Run completed with %d output(s).", len(result.Outputs))
	}
	if result.Warning != "" {
		model.status += " " + result.Warning
	}
}

func (model *Model) finishFailedRun(message RunFailedMsg) tea.Cmd {
	model.active = core.RequestMeta{}
	model.cancelling = false
	model.stale = false
	var structured *core.Error
	cancelled := errors.As(message.Err, &structured) && structured.Category == core.ErrorCancellation
	if len(message.Partial.Outputs) > 0 {
		model.result = message.Partial
		model.resultSnapshot = model.lastRun.Snapshot
		model.resetResultNavigation()
	}
	if cancelled {
		if len(message.Partial.Outputs) > 0 {
			for index := range model.result.Outputs {
				if model.result.Outputs[index].Kind == OutputRows && index == len(model.result.Outputs)-1 {
					model.result.Outputs[index].Incomplete = true
				}
			}
		}
		model.lifecycle = LifecycleIdle
		model.status = "Run cancelled. Completed output is retained; rerun remains available."
	} else {
		model.result = message.Partial
		model.resultSnapshot = model.lastRun.Snapshot
		model.resetResultNavigation()
		if structured == nil {
			structured = core.NewError("execute SQL", core.ErrorInternal, "The SQL run failed.", true, message.Err)
		}
		model.result.Outputs = append(model.result.Outputs, Output{Kind: OutputError, Error: structured})
		model.lifecycle = LifecycleFailed
		model.status = "SQL execution failed. Earlier outputs were preserved."
	}
	if message.Partial.Warning != "" {
		model.status += " " + message.Partial.Warning
	}
	model.pane = PaneResults
	model.textMode = false
	model.editor.Blur()
	return nil
}

func (model *Model) enterTextMode() {
	model.textMode = true
	model.pane = PaneEditor
	model.editor.Focus()
	model.status = model.executionTargetStatus()
}

func (model *Model) resetResultNavigation() {
	model.activeOutput = 0
	model.resultPage = 0
	model.selectedRow = 0
	model.resultRowOffset = 0
	model.selectedColumn = 0
	model.columnOffset = 0
	model.selectedRows = make(map[int]bool)
	model.inspector = false
}

func (model *Model) moveOutput(delta int) {
	if len(model.result.Outputs) == 0 {
		return
	}
	model.activeOutput = (model.activeOutput + delta + len(model.result.Outputs)) % len(model.result.Outputs)
	model.resultPage = 0
	model.selectedRow = 0
	model.resultRowOffset = 0
	model.selectedColumn = 0
	model.columnOffset = 0
	model.selectedRows = make(map[int]bool)
}

func (model *Model) moveResultPage(delta int) {
	output, ok := model.currentOutput()
	if !ok || output.Kind != OutputRows {
		return
	}
	pages := max(1, (len(output.Rows)+ResultPageSize-1)/ResultPageSize)
	target := min(pages-1, max(0, model.resultPage+delta))
	if target == model.resultPage {
		if delta > 0 {
			model.status = "Already on the last result page."
		} else {
			model.status = "Already on the first result page."
		}
		return
	}
	model.resultPage = target
	model.selectedRow = target * ResultPageSize
	model.resultRowOffset = model.selectedRow
}

func (model *Model) moveResultRow(delta int) {
	output, ok := model.currentOutput()
	if !ok || output.Kind != OutputRows || len(output.Rows) == 0 {
		return
	}
	pageStart, pageEnd := model.pageBounds(output)
	model.selectedRow = min(pageEnd-1, max(pageStart, model.selectedRow+delta))
	model.ensureResultRowVisible(output)
}

func (model *Model) moveResultColumn(delta int) {
	output, ok := model.currentOutput()
	if !ok || output.Kind != OutputRows || len(output.Columns) == 0 {
		return
	}
	model.selectedColumn = min(len(output.Columns)-1, max(0, model.selectedColumn+delta))
	model.ensureResultColumnVisible(output)
}

func (model *Model) toggleSelectedRow() {
	output, ok := model.currentOutput()
	if !ok || output.Kind != OutputRows || len(output.Rows) == 0 {
		return
	}
	if model.selectedRows[model.selectedRow] {
		delete(model.selectedRows, model.selectedRow)
	} else {
		model.selectedRows[model.selectedRow] = true
	}
}

func (model Model) currentOutput() (Output, bool) {
	if model.activeOutput < 0 || model.activeOutput >= len(model.result.Outputs) {
		return Output{}, false
	}
	return model.result.Outputs[model.activeOutput], true
}

func (model *Model) clampResultSelection() {
	output, ok := model.currentOutput()
	if !ok || output.Kind != OutputRows || len(output.Rows) == 0 {
		model.selectedRow = 0
		model.selectedColumn = 0
		return
	}
	model.selectedRow = min(len(output.Rows)-1, max(0, model.selectedRow))
	model.selectedColumn = min(max(0, len(output.Columns)-1), max(0, model.selectedColumn))
	model.ensureResultRowVisible(output)
}

func (model Model) pageBounds(output Output) (int, int) {
	start := min(len(output.Rows), model.resultPage*ResultPageSize)
	return start, min(len(output.Rows), start+ResultPageSize)
}

func (model Model) visibleResultRowCount() int {
	return max(1, model.height-(model.resultTop()+3))
}

func (model *Model) ensureResultRowVisible(output Output) {
	pageStart, pageEnd := model.pageBounds(output)
	if pageEnd <= pageStart {
		model.resultRowOffset = pageStart
		return
	}
	visible := model.visibleResultRowCount()
	model.resultRowOffset = max(pageStart, model.resultRowOffset)
	if model.selectedRow < model.resultRowOffset {
		model.resultRowOffset = model.selectedRow
	}
	if model.selectedRow >= model.resultRowOffset+visible {
		model.resultRowOffset = model.selectedRow - visible + 1
	}
	model.resultRowOffset = min(max(pageStart, pageEnd-visible), model.resultRowOffset)
}

func (model *Model) openInspector() {
	output, ok := model.currentOutput()
	if !ok || output.Kind != OutputRows || model.selectedRow >= len(output.Rows) ||
		model.selectedColumn >= len(output.Rows[model.selectedRow]) {
		return
	}
	model.inspector = true
	model.inspectorScroll = 0
}

func (model *Model) updateInspectorKey(key string) tea.Cmd {
	switch key {
	case "esc", "enter":
		model.inspector = false
	case "up", "k", "pgup":
		model.inspectorScroll = max(0, model.inspectorScroll-1)
	case "down", "j", "pgdown":
		model.inspectorScroll++
	case "c":
		return model.copySelection(false)
	}
	return nil
}

func (model Model) executionTargetStatus() string {
	if model.hasSelection() {
		return "Editing SQL. Execute targets the selection; Run all targets the buffer."
	}
	return "Editing SQL. Execute targets the full buffer."
}

func messageCommand(message tea.Msg) tea.Cmd {
	return func() tea.Msg { return message }
}
