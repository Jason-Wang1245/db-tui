package grid

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

type Lifecycle string

const (
	LifecycleIdle    Lifecycle = "idle"
	LifecycleRunning Lifecycle = "running"
	LifecycleFailed  Lifecycle = "failed"
)

type operationKind string

const (
	operationDescribe operationKind = "describe"
	operationPage     operationKind = "page"
)

type DescribeIntent struct {
	Relation RelationID
	Meta     core.RequestMeta
}

type LoadPageIntent struct {
	Relation Relation
	Page     PageRequest
	Meta     core.RequestMeta
}

type CancelIntent struct {
	Operation core.OperationID
}

type RelationDescribedMsg struct {
	Relation Relation
	Meta     core.RequestMeta
}

type PageLoadedMsg struct {
	Page Page
	Meta core.RequestMeta
}

type OperationFailedMsg struct {
	Kind operationKind
	Err  error
	Meta core.RequestMeta
}

type activeOperation struct {
	Kind operationKind
	Meta core.RequestMeta
}

type Model struct {
	workspace      core.WorkspaceID
	tab            core.TabID
	relationID     RelationID
	relation       *Relation
	page           Page
	sort           Sort
	pageNumber     int
	pendingPage    int
	pendingRequest PageRequest
	width          int
	height         int
	selectedRow    int
	selectedColumn int
	rowOffset      int
	columnOffset   int
	columnWidths   []int
	nextRequest    core.RequestID
	active         activeOperation
	lifecycle      Lifecycle
	status         string
	err            error
}

func NewModel(workspace core.WorkspaceID, tab core.TabID, relation RelationID) Model {
	return Model{
		workspace: workspace, tab: tab, relationID: relation, nextRequest: 1,
		lifecycle: LifecycleIdle, status: "Preparing table metadata…",
	}
}

func (model *Model) Init() tea.Cmd {
	return model.describe()
}

func (model *Model) SetSize(width, height int) {
	model.width = width
	model.height = height
	model.ensureRowVisible()
	model.ensureColumnVisible()
}

func (model Model) Tab() core.TabID              { return model.tab }
func (model Model) Busy() bool                   { return model.active.Meta.Operation != "" }
func (model Model) ActiveMeta() core.RequestMeta { return model.active.Meta }
func (model Model) Lifecycle() Lifecycle         { return model.lifecycle }
func (model Model) Relation() (Relation, bool) {
	if model.relation == nil {
		return Relation{}, false
	}
	return *model.relation, true
}

func (model *Model) Update(message tea.Msg) tea.Cmd {
	switch message := message.(type) {
	case RelationDescribedMsg:
		if !model.matches(operationDescribe, message.Meta) {
			return nil
		}
		model.clearActive()
		relation := message.Relation
		model.relation = &relation
		model.selectedColumn = 0
		model.err = nil
		model.status = "Loading the first page…"
		return model.startPage(PageRequest{
			Relation: relation.ID, Direction: PageFirst, Limit: DefaultPageSize,
		}, 1)
	case PageLoadedMsg:
		if !model.matches(operationPage, message.Meta) {
			return nil
		}
		model.clearActive()
		model.page = message.Page
		model.pageNumber = max(1, model.pendingPage)
		model.selectedRow = 0
		model.rowOffset = 0
		model.err = nil
		model.lifecycle = LifecycleIdle
		model.computeColumnWidths()
		model.ensureColumnVisible()
		if len(message.Page.Rows) == 0 {
			model.status = "No rows on this page."
		} else {
			model.status = fmt.Sprintf("Loaded %d rows on page %d.", len(message.Page.Rows), model.pageNumber)
		}
	case OperationFailedMsg:
		if !model.matches(message.Kind, message.Meta) {
			return nil
		}
		model.clearActive()
		model.err = message.Err
		model.lifecycle = LifecycleFailed
		model.status = "Browsing failed. Press r to retry."
		var structured *core.Error
		if errors.As(message.Err, &structured) && structured.Category == core.ErrorCancellation {
			model.err = nil
			model.lifecycle = LifecycleIdle
			model.status = "Operation cancelled. You can run it again."
		}
	case tea.KeyPressMsg:
		return model.updateKey(message.Keystroke())
	case tea.MouseClickMsg:
		return model.updateMouse(message.Mouse())
	case tea.MouseWheelMsg:
		return model.updateWheel(message.Mouse())
	}
	return nil
}

func (model *Model) updateKey(key string) tea.Cmd {
	if model.Busy() {
		if key == "ctrl+c" {
			operation := model.active.Meta.Operation
			model.status = "Cancelling table operation…"
			return func() tea.Msg { return CancelIntent{Operation: operation} }
		}
		return nil
	}
	switch key {
	case "up", "k":
		model.moveRow(-1)
	case "down", "j":
		model.moveRow(1)
	case "left", "h":
		model.moveColumn(-1)
	case "right", "l":
		model.moveColumn(1)
	case "home", "g":
		model.selectedRow = 0
		model.rowOffset = 0
	case "end", "G":
		if len(model.page.Rows) > 0 {
			model.selectedRow = len(model.page.Rows) - 1
			model.ensureRowVisible()
		}
	case "n", "pgdown":
		if model.page.NextCursor != "" {
			return model.startPage(PageRequest{
				Relation: model.relationID, Cursor: model.page.NextCursor, Direction: PageNext,
				Sort: model.sort, Limit: DefaultPageSize,
			}, model.pageNumber+1)
		}
		model.status = "Already on the last page."
	case "p", "pgup":
		if model.page.PrevCursor != "" {
			return model.startPage(PageRequest{
				Relation: model.relationID, Cursor: model.page.PrevCursor, Direction: PagePrevious,
				Sort: model.sort, Limit: DefaultPageSize,
			}, max(1, model.pageNumber-1))
		}
		model.status = "Already on the first page."
	case "r":
		if model.err != nil {
			return model.retry()
		}
		if model.relation == nil {
			return model.describe()
		}
		return model.startPage(PageRequest{
			Relation: model.relationID, Direction: PageFirst, Sort: model.sort, Limit: DefaultPageSize,
		}, 1)
	case "s":
		return model.cycleSort()
	}
	return nil
}

func (model *Model) describe() tea.Cmd {
	request := model.newRequest()
	meta := model.meta(operationDescribe, request)
	model.active = activeOperation{Kind: operationDescribe, Meta: meta}
	model.lifecycle = LifecycleRunning
	model.err = nil
	model.status = "Loading table metadata…"
	intent := DescribeIntent{Relation: model.relationID, Meta: meta}
	return func() tea.Msg { return intent }
}

func (model *Model) startPage(page PageRequest, targetPage int) tea.Cmd {
	if model.relation == nil {
		return model.describe()
	}
	request := model.newRequest()
	meta := model.meta(operationPage, request)
	model.active = activeOperation{Kind: operationPage, Meta: meta}
	model.lifecycle = LifecycleRunning
	model.err = nil
	model.pendingRequest = page
	model.pendingPage = targetPage
	model.status = fmt.Sprintf("Loading page %d…", targetPage)
	intent := LoadPageIntent{Relation: *model.relation, Page: page, Meta: meta}
	return func() tea.Msg { return intent }
}

func (model *Model) retry() tea.Cmd {
	if model.relation == nil || model.pendingRequest.Relation == (RelationID{}) {
		return model.describe()
	}
	return model.startPage(model.pendingRequest, max(1, model.pendingPage))
}

func (model *Model) cycleSort() tea.Cmd {
	if model.relation == nil || len(model.relation.Columns) == 0 {
		return nil
	}
	column := model.relation.Columns[model.selectedColumn]
	if !column.Sortable {
		model.status = column.Name + " cannot be sorted by PostgreSQL."
		return nil
	}
	if model.sort.Column != column.Name {
		model.sort = Sort{Column: column.Name, Ascending: true}
	} else if model.sort.Ascending {
		model.sort.Ascending = false
	} else {
		model.sort = Sort{}
	}
	return model.startPage(PageRequest{
		Relation: model.relationID, Direction: PageFirst, Sort: model.sort, Limit: DefaultPageSize,
	}, 1)
}

func (model *Model) moveRow(delta int) {
	if len(model.page.Rows) == 0 {
		return
	}
	model.selectedRow = min(len(model.page.Rows)-1, max(0, model.selectedRow+delta))
	model.ensureRowVisible()
}

func (model *Model) moveColumn(delta int) {
	if model.relation == nil || len(model.relation.Columns) == 0 {
		return
	}
	model.selectedColumn = min(len(model.relation.Columns)-1, max(0, model.selectedColumn+delta))
	model.ensureColumnVisible()
}

func (model *Model) ensureRowVisible() {
	visible := model.visibleRowCount()
	if model.selectedRow < model.rowOffset {
		model.rowOffset = model.selectedRow
	}
	if model.selectedRow >= model.rowOffset+visible {
		model.rowOffset = model.selectedRow - visible + 1
	}
	model.rowOffset = min(max(0, len(model.page.Rows)-visible), max(0, model.rowOffset))
}

func (model *Model) ensureColumnVisible() {
	if model.relation == nil || len(model.relation.Columns) == 0 || len(model.columnWidths) == 0 {
		model.columnOffset = 0
		return
	}
	if model.selectedColumn < model.columnOffset {
		model.columnOffset = model.selectedColumn
	}
	available := max(1, model.width-2)
	for model.columnOffset < model.selectedColumn {
		used := 0
		for index := model.columnOffset; index <= model.selectedColumn; index++ {
			if index > model.columnOffset {
				used++
			}
			used += model.columnWidths[index]
		}
		if used <= available {
			break
		}
		model.columnOffset++
	}
}

func (model Model) visibleRowCount() int {
	return max(1, model.height-5)
}

func (model *Model) computeColumnWidths() {
	if model.relation == nil {
		model.columnWidths = nil
		return
	}
	model.columnWidths = make([]int, len(model.relation.Columns))
	for index, column := range model.relation.Columns {
		width := displayWidth(column.Name)
		if column.IdentityPart {
			width = max(width, displayWidth(column.Name+" [key]"))
		}
		for _, row := range model.page.Rows {
			if index < len(row.Cells) {
				width = max(width, displayWidth(row.Cells[index].Display))
			}
		}
		model.columnWidths[index] = min(36, max(4, width+2))
	}
}

func (model *Model) clearActive() {
	model.active = activeOperation{}
}

func (model *Model) newRequest() core.RequestID {
	request := model.nextRequest
	model.nextRequest++
	return request
}

func (model Model) meta(kind operationKind, request core.RequestID) core.RequestMeta {
	return core.RequestMeta{
		Workspace: model.workspace,
		Tab:       model.tab,
		Operation: core.OperationID(fmt.Sprintf("grid.%s.%s.%d", kind, model.tab, request)),
		Request:   request,
	}
}

func (model Model) matches(kind operationKind, meta core.RequestMeta) bool {
	return model.active.Kind == kind && model.active.Meta.Matches(meta)
}

func DescribeFailed(meta core.RequestMeta, err error) OperationFailedMsg {
	return OperationFailedMsg{Kind: operationDescribe, Err: err, Meta: meta}
}

func PageFailed(meta core.RequestMeta, err error) OperationFailedMsg {
	return OperationFailedMsg{Kind: operationPage, Err: err, Meta: meta}
}
