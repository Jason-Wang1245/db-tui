package workspace

import (
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

const healthCheckInterval = 10 * time.Second

type Focus string

const (
	FocusNavigator Focus = "workspace.navigator"
	FocusTabs      Focus = "workspace.tabs"
	FocusContent   Focus = "workspace.content"
)

type LayoutMode string

const (
	LayoutWide   LayoutMode = "wide"
	LayoutDrawer LayoutMode = "drawer"
	LayoutSingle LayoutMode = "single"
)

type ConnectionState string

const (
	ConnectionConnected    ConnectionState = "connected"
	ConnectionLost         ConnectionState = "lost"
	ConnectionReconnecting ConnectionState = "reconnecting"
)

type modalKind string

const (
	modalNone       modalKind = ""
	modalCloseTab   modalKind = "close-tab"
	modalDisconnect modalKind = "disconnect"
	modalQuit       modalKind = "quit"
)

type modalState struct {
	Kind        modalKind
	Tab         core.TabID
	Destructive bool
}

type treeSchema struct {
	Schema
	Expanded  bool
	Loaded    bool
	Loading   bool
	Request   core.RequestID
	Relations []Relation
}

type treeItemKind string

const (
	treeSchemaItem   treeItemKind = "schema"
	treeRelationItem treeItemKind = "relation"
)

type treeItem struct {
	Kind     treeItemKind
	Schema   int
	Relation int
}

type operationKind string

const (
	operationSchemas   operationKind = "schemas"
	operationRelations operationKind = "relations"
	operationHealth    operationKind = "health"
	operationReconnect operationKind = "reconnect"
)

type LoadSchemasIntent struct {
	Meta core.RequestMeta
}

type LoadRelationsIntent struct {
	Schema string
	Meta   core.RequestMeta
}

type CheckConnectionIntent struct {
	Meta core.RequestMeta
}

type ReconnectIntent struct {
	Meta core.RequestMeta
}

type OpenTableIntent struct {
	Workspace core.WorkspaceID
	Tab       core.TabID
	Relation  Relation
}

type CancelIntent struct {
	Operations []core.OperationID
}

type DisconnectIntent struct{}
type QuitIntent struct{}

type SchemasLoadedMsg struct {
	Schemas []Schema
	Meta    core.RequestMeta
}

type RelationsLoadedMsg struct {
	Schema    string
	Relations []Relation
	Meta      core.RequestMeta
}

type ConnectionCheckedMsg struct {
	Meta core.RequestMeta
}

type ReconnectedMsg struct {
	Meta core.RequestMeta
}

// TabStateChangedMsg is the shell seam used by table and SQL reducers to keep
// close/disconnect/quit behavior safe without exposing their local state.
type TabStateChangedMsg struct {
	Tab       core.TabID
	Dirty     bool
	DirtySet  bool
	Lifecycle TabLifecycle
	Request   core.RequestMeta
}

type OperationFailedMsg struct {
	Kind   operationKind
	Schema string
	Err    error
	Meta   core.RequestMeta
}

type healthCheckDueMsg struct {
	Workspace  core.WorkspaceID
	Generation uint64
}

// CheckConnectionNow returns the same message used by the periodic health
// timer. It is also a deterministic seam for root lifecycle tests.
func (model Model) CheckConnectionNow() tea.Msg {
	return healthCheckDueMsg{Workspace: model.id, Generation: model.healthGeneration}
}

type Model struct {
	id               core.WorkspaceID
	profileName      string
	database         string
	server           string
	serverVersion    string
	width            int
	height           int
	layout           LayoutMode
	focus            Focus
	previousFocus    Focus
	help             bool
	schemas          []treeSchema
	selectedTree     int
	treeOffset       int
	tabs             []Tab
	activeTab        core.TabID
	nextTab          uint64
	nextSQL          uint64
	nextRequest      core.RequestID
	schemasRequest   core.RequestID
	healthRequest    core.RequestID
	reconnectRequest core.RequestID
	healthGeneration uint64
	connection       ConnectionState
	status           string
	err              error
	warning          *core.Error
	hitboxes         []Hitbox
	modal            modalState
}

func NewModel(id core.WorkspaceID, profileName, database, server, serverVersion string, warning *core.Error) Model {
	return Model{
		id: id, profileName: profileName, database: database, server: server,
		serverVersion: serverVersion, focus: FocusNavigator, previousFocus: FocusNavigator,
		nextTab: 1, nextSQL: 1, nextRequest: 1, connection: ConnectionConnected,
		healthGeneration: 1, status: "Loading database objects…", warning: warning,
	}
}

func (model *Model) SetSize(width, height int) {
	model.width = width
	model.height = height
	model.layout = LayoutFor(width, height)
	model.ensureTreeVisible()
	model.rebuildHitboxes()
}

func LayoutFor(width, height int) LayoutMode {
	if width >= 100 && height >= 24 {
		return LayoutWide
	}
	if width >= 70 && height >= 16 {
		return LayoutDrawer
	}
	return LayoutSingle
}

func (model *Model) Init() tea.Cmd {
	request := model.newRequest()
	model.schemasRequest = request
	intent := LoadSchemasIntent{Meta: model.meta(operationSchemas, request, "")}
	return func() tea.Msg { return intent }
}

func (model Model) ID() core.WorkspaceID        { return model.id }
func (model Model) Focus() Focus                { return model.focus }
func (model Model) Layout() LayoutMode          { return model.layout }
func (model Model) Connection() ConnectionState { return model.connection }

func (model Model) Tabs() []Tab {
	return append([]Tab(nil), model.tabs...)
}

func (model Model) ActiveTab() core.TabID { return model.activeTab }

func (model Model) ActiveTable() (core.TabID, Relation, bool) {
	index := model.activeTabIndex()
	if index < 0 || model.tabs[index].Table == nil {
		return "", Relation{}, false
	}
	return model.tabs[index].Envelope.ID, model.tabs[index].Table.Relation, true
}

func (model Model) ContentRect() ui.Rect {
	x := 0
	width := max(0, model.width)
	if model.layout == LayoutWide {
		x = workspaceNavigatorWidth(model.width) + 1
		width = max(0, model.width-x)
	}
	return ui.Rect{X: x, Y: 2, Width: width, Height: max(0, model.height-4)}
}

func (model Model) RoutesToContent(message tea.Msg) bool {
	if model.connection != ConnectionConnected || model.help || model.modal.Kind != modalNone {
		return false
	}
	if _, _, ok := model.ActiveTable(); !ok {
		return false
	}
	switch message := message.(type) {
	case tea.KeyPressMsg:
		if model.focus != FocusContent {
			return false
		}
		switch message.Keystroke() {
		case "tab", "shift+tab", "[", "]", "?", "q", "ctrl+d", "esc":
			return false
		default:
			return true
		}
	case tea.MouseClickMsg:
		if model.layout != LayoutWide && model.focus == FocusNavigator {
			return false
		}
		mouse := message.Mouse()
		return model.ContentRect().Contains(mouse.X, mouse.Y)
	case tea.MouseWheelMsg:
		if model.layout != LayoutWide && model.focus == FocusNavigator {
			return false
		}
		mouse := message.Mouse()
		return model.ContentRect().Contains(mouse.X, mouse.Y)
	}
	return false
}

func (model Model) ExpectsReconnect(meta core.RequestMeta) bool {
	return model.matches(operationReconnect, meta, model.reconnectRequest, "")
}

func (model Model) Busy() bool {
	if model.schemasRequest != 0 || model.healthRequest != 0 || model.reconnectRequest != 0 {
		return true
	}
	for _, schema := range model.schemas {
		if schema.Loading {
			return true
		}
	}
	return false
}

func (model *Model) Update(message tea.Msg) tea.Cmd {
	switch message := message.(type) {
	case SchemasLoadedMsg:
		if !model.matches(operationSchemas, message.Meta, model.schemasRequest, "") {
			return nil
		}
		model.schemasRequest = 0
		model.schemas = make([]treeSchema, 0, len(message.Schemas))
		for _, schema := range message.Schemas {
			model.schemas = append(model.schemas, treeSchema{Schema: schema})
		}
		model.selectedTree = 0
		model.treeOffset = 0
		model.err = nil
		model.status = fmt.Sprintf("Loaded %d schemas.", len(model.schemas))
		model.rebuildHitboxes()
		return scheduleHealthCheck(model.id, model.healthGeneration)
	case RelationsLoadedMsg:
		index := model.schemaIndex(message.Schema)
		if index < 0 || !model.matches(operationRelations, message.Meta, model.schemas[index].Request, message.Schema) {
			return nil
		}
		model.schemas[index].Relations = append([]Relation(nil), message.Relations...)
		model.schemas[index].Loading = false
		model.schemas[index].Loaded = true
		model.schemas[index].Request = 0
		model.err = nil
		model.status = fmt.Sprintf("Loaded %d objects from %s.", len(message.Relations), message.Schema)
		model.ensureTreeVisible()
		model.rebuildHitboxes()
	case ConnectionCheckedMsg:
		if !model.matches(operationHealth, message.Meta, model.healthRequest, "") {
			return nil
		}
		model.healthRequest = 0
		if model.connection == ConnectionConnected {
			return scheduleHealthCheck(model.id, model.healthGeneration)
		}
	case ReconnectedMsg:
		if !model.matches(operationReconnect, message.Meta, model.reconnectRequest, "") {
			return nil
		}
		model.reconnectRequest = 0
		model.schemasRequest = 0
		model.healthRequest = 0
		for index := range model.schemas {
			model.schemas[index].Loading = false
			model.schemas[index].Request = 0
		}
		model.connection = ConnectionConnected
		model.healthGeneration++
		model.err = nil
		refresh := model.refreshSchemas()
		model.status = "Reconnected. Open tabs were preserved; no work was replayed."
		return tea.Batch(refresh, scheduleHealthCheck(model.id, model.healthGeneration))
	case TabStateChangedMsg:
		for index := range model.tabs {
			if model.tabs[index].Envelope.ID == message.Tab {
				if message.DirtySet {
					model.tabs[index].Envelope.Dirty = message.Dirty
				}
				model.tabs[index].Envelope.Lifecycle = message.Lifecycle
				model.tabs[index].Envelope.ActiveRequest = message.Request
				break
			}
		}
		model.rebuildHitboxes()
	case OperationFailedMsg:
		return model.handleFailure(message)
	case healthCheckDueMsg:
		if message.Workspace != model.id || message.Generation != model.healthGeneration ||
			model.connection != ConnectionConnected || model.healthRequest != 0 {
			return nil
		}
		request := model.newRequest()
		model.healthRequest = request
		intent := CheckConnectionIntent{Meta: model.meta(operationHealth, request, "")}
		return func() tea.Msg { return intent }
	case tea.MouseClickMsg:
		if model.help {
			model.help = false
			model.focus = model.previousFocus
			model.rebuildHitboxes()
			return nil
		}
		return model.updateMouse(message.Mouse())
	case tea.MouseWheelMsg:
		if model.help || model.modal.Kind != modalNone {
			return nil
		}
		return model.updateWheel(message.Mouse())
	case tea.KeyPressMsg:
		return model.updateKey(message.Keystroke())
	}
	return nil
}

func (model *Model) updateKey(key string) tea.Cmd {
	if model.modal.Kind != modalNone {
		return model.updateModalKey(key)
	}
	if model.help {
		if key == "esc" || key == "?" || key == "enter" {
			model.help = false
			model.focus = model.previousFocus
		}
		return nil
	}
	if key == "?" {
		model.previousFocus = model.focus
		model.help = true
		return nil
	}
	if key == "ctrl+c" && model.Busy() {
		return model.cancelActive()
	}
	if key == "ctrl+c" {
		return model.requestLeave(modalQuit)
	}
	if model.connection == ConnectionLost {
		switch key {
		case "r", "enter":
			return model.reconnect()
		case "d", "esc":
			return model.requestLeave(modalDisconnect)
		case "q":
			return model.requestLeave(modalQuit)
		}
		return nil
	}
	switch key {
	case "tab":
		model.moveFocus(1)
	case "shift+tab":
		model.moveFocus(-1)
	case "[":
		model.moveTab(-1)
	case "]":
		model.moveTab(1)
	case "q":
		return model.requestLeave(modalQuit)
	case "ctrl+d":
		return model.requestLeave(modalDisconnect)
	case "esc":
		if model.focus == FocusContent {
			model.focus = FocusTabs
		} else if model.focus == FocusNavigator && model.layout != LayoutWide {
			model.focus = FocusTabs
		}
	default:
		return model.updateFocusedKey(key)
	}
	model.rebuildHitboxes()
	return nil
}

func (model *Model) updateFocusedKey(key string) tea.Cmd {
	switch model.focus {
	case FocusNavigator:
		switch key {
		case "up", "k":
			model.moveTree(-1)
		case "down", "j":
			model.moveTree(1)
		case "left", "h":
			model.collapseSelected()
		case "right", "l", "space":
			return model.expandSelected()
		case "enter":
			return model.activateTreeItem()
		case "r":
			return model.refreshSchemas()
		}
	case FocusTabs:
		switch key {
		case "left", "h":
			model.moveTab(-1)
		case "right", "l":
			model.moveTab(1)
		case "n":
			model.openSQLTab()
		case "x":
			return model.closeActiveTab()
		case "enter":
			if model.activeTab != "" {
				model.focus = FocusContent
				model.rememberFocus()
			}
		}
	case FocusContent:
		// Kind-specific reducers own content navigation. The shell handles only
		// the global keys above while content is focused.
	}
	model.rebuildHitboxes()
	return nil
}

func (model *Model) activateTreeItem() tea.Cmd {
	item, ok := model.selectedItem()
	if !ok {
		return nil
	}
	if item.Kind == treeSchemaItem {
		return model.toggleSchema(item.Schema)
	}
	relation := model.schemas[item.Schema].Relations[item.Relation]
	return model.openTableTab(relation)
}

func (model *Model) expandSelected() tea.Cmd {
	item, ok := model.selectedItem()
	if !ok || item.Kind != treeSchemaItem {
		return nil
	}
	if model.schemas[item.Schema].Expanded {
		return nil
	}
	return model.toggleSchema(item.Schema)
}

func (model *Model) collapseSelected() {
	item, ok := model.selectedItem()
	if !ok {
		return
	}
	if item.Kind == treeRelationItem {
		items := model.visibleItems()
		for index, candidate := range items {
			if candidate.Kind == treeSchemaItem && candidate.Schema == item.Schema {
				model.selectedTree = index
				break
			}
		}
		return
	}
	model.schemas[item.Schema].Expanded = false
	model.ensureTreeVisible()
}

func (model *Model) toggleSchema(index int) tea.Cmd {
	schema := &model.schemas[index]
	schema.Expanded = !schema.Expanded
	if !schema.Expanded || schema.Loaded || schema.Loading {
		model.ensureTreeVisible()
		model.rebuildHitboxes()
		return nil
	}
	request := model.newRequest()
	schema.Loading = true
	schema.Request = request
	model.status = "Loading " + schema.Name + "…"
	intent := LoadRelationsIntent{Schema: schema.Name, Meta: model.meta(operationRelations, request, schema.Name)}
	model.rebuildHitboxes()
	return func() tea.Msg { return intent }
}

func (model *Model) refreshSchemas() tea.Cmd {
	if model.schemasRequest != 0 {
		return nil
	}
	request := model.newRequest()
	model.schemasRequest = request
	model.status = "Refreshing database objects…"
	intent := LoadSchemasIntent{Meta: model.meta(operationSchemas, request, "")}
	return func() tea.Msg { return intent }
}

func (model *Model) reconnect() tea.Cmd {
	if model.reconnectRequest != 0 {
		return nil
	}
	request := model.newRequest()
	model.reconnectRequest = request
	model.connection = ConnectionReconnecting
	model.status = "Reconnecting…"
	intent := ReconnectIntent{Meta: model.meta(operationReconnect, request, "")}
	return func() tea.Msg { return intent }
}

func (model *Model) cancelActive() tea.Cmd {
	operations := make([]core.OperationID, 0, 4)
	if model.schemasRequest != 0 {
		operations = append(operations, model.meta(operationSchemas, model.schemasRequest, "").Operation)
	}
	if model.healthRequest != 0 {
		operations = append(operations, model.meta(operationHealth, model.healthRequest, "").Operation)
	}
	if model.reconnectRequest != 0 {
		operations = append(operations, model.meta(operationReconnect, model.reconnectRequest, "").Operation)
	}
	for _, schema := range model.schemas {
		if schema.Request != 0 {
			operations = append(operations, model.meta(operationRelations, schema.Request, schema.Name).Operation)
		}
	}
	model.status = "Cancelling active workspace operation…"
	return messageCommand(CancelIntent{Operations: operations})
}

func (model *Model) handleFailure(message OperationFailedMsg) tea.Cmd {
	if message.Meta.Workspace != model.id {
		return nil
	}
	coreError := asCoreError(message.Err)
	cancelled := coreError != nil && coreError.Category == core.ErrorCancellation
	switch message.Kind {
	case operationSchemas:
		if !model.matches(operationSchemas, message.Meta, model.schemasRequest, "") {
			return nil
		}
		model.schemasRequest = 0
	case operationRelations:
		index := model.schemaIndex(message.Schema)
		if index < 0 || !model.matches(operationRelations, message.Meta, model.schemas[index].Request, message.Schema) {
			return nil
		}
		model.schemas[index].Loading = false
		model.schemas[index].Request = 0
	case operationHealth:
		if !model.matches(operationHealth, message.Meta, model.healthRequest, "") {
			return nil
		}
		model.healthRequest = 0
		if !cancelled {
			model.connection = ConnectionLost
			model.status = "Connection lost. Press r to reconnect or d to disconnect."
		}
	case operationReconnect:
		if !model.matches(operationReconnect, message.Meta, model.reconnectRequest, "") {
			return nil
		}
		model.reconnectRequest = 0
		model.connection = ConnectionLost
		model.status = "Reconnect failed. Press r to try again or d to disconnect."
	}
	model.err = message.Err
	if cancelled {
		model.status = "Operation cancelled. You can run it again."
		model.err = nil
	} else if coreError != nil && coreError.Category == core.ErrorNetwork {
		model.connection = ConnectionLost
		model.status = "Connection lost. Press r to reconnect or d to disconnect."
	}
	return nil
}

func asCoreError(err error) *core.Error {
	var result *core.Error
	if errors.As(err, &result) {
		return result
	}
	return nil
}

func SchemasFailed(meta core.RequestMeta, err error) OperationFailedMsg {
	return OperationFailedMsg{Kind: operationSchemas, Err: err, Meta: meta}
}

func RelationsFailed(schema string, meta core.RequestMeta, err error) OperationFailedMsg {
	return OperationFailedMsg{Kind: operationRelations, Schema: schema, Err: err, Meta: meta}
}

func HealthCheckFailed(meta core.RequestMeta, err error) OperationFailedMsg {
	return OperationFailedMsg{Kind: operationHealth, Err: err, Meta: meta}
}

func ReconnectFailed(meta core.RequestMeta, err error) OperationFailedMsg {
	return OperationFailedMsg{Kind: operationReconnect, Err: err, Meta: meta}
}

func (model *Model) openTableTab(relation Relation) tea.Cmd {
	for _, tab := range model.tabs {
		if tab.Table != nil && tab.Table.Relation.Schema == relation.Schema && tab.Table.Relation.Name == relation.Name {
			model.activeTab = tab.Envelope.ID
			model.focus = tab.Envelope.LastFocus
			if model.focus == "" {
				model.focus = FocusContent
			}
			model.status = "Focused existing table tab."
			model.rebuildHitboxes()
			return nil
		}
	}
	id := core.TabID(fmt.Sprintf("table-%d", model.nextTab))
	model.nextTab++
	tab := Tab{
		Envelope: TabEnvelope{ID: id, Title: relation.Name, Kind: TabTable, Lifecycle: TabIdle, LastFocus: FocusContent},
		Table:    &TableTabState{Relation: relation},
	}
	model.tabs = append(model.tabs, tab)
	model.activeTab = id
	model.focus = FocusContent
	model.status = "Opened " + relation.Schema + "." + relation.Name + "."
	model.rebuildHitboxes()
	intent := OpenTableIntent{Workspace: model.id, Tab: id, Relation: relation}
	return func() tea.Msg { return intent }
}

func (model *Model) openSQLTab() {
	label := fmt.Sprintf("SQL %d", model.nextSQL)
	model.nextSQL++
	id := core.TabID(fmt.Sprintf("sql-%d", model.nextTab))
	model.nextTab++
	model.tabs = append(model.tabs, Tab{
		Envelope: TabEnvelope{ID: id, Title: label, Kind: TabSQL, Lifecycle: TabIdle, LastFocus: FocusContent},
		SQL:      &SQLTabState{},
	})
	model.activeTab = id
	model.focus = FocusContent
	model.status = "Opened " + label + "."
	model.rebuildHitboxes()
}

func (model *Model) closeActiveTab() tea.Cmd {
	index := model.activeTabIndex()
	if index < 0 {
		return nil
	}
	tab := model.tabs[index]
	if tab.Envelope.Dirty || tab.Envelope.Lifecycle == TabRunning {
		model.previousFocus = model.focus
		model.modal = modalState{Kind: modalCloseTab, Tab: tab.Envelope.ID}
		model.status = "Choose whether to discard this tab's local work."
		model.rebuildHitboxes()
		return nil
	}
	model.closeTabAt(index)
	return nil
}

func (model *Model) closeTabAt(index int) {
	closed := model.tabs[index].Envelope.Title
	model.tabs = append(model.tabs[:index], model.tabs[index+1:]...)
	if len(model.tabs) == 0 {
		model.activeTab = ""
		model.focus = FocusTabs
	} else {
		if index >= len(model.tabs) {
			index = len(model.tabs) - 1
		}
		model.activeTab = model.tabs[index].Envelope.ID
	}
	model.status = "Closed " + closed + "."
	model.rebuildHitboxes()
}

func (model *Model) requestLeave(kind modalKind) tea.Cmd {
	if !model.hasHazardousTabs() {
		if kind == modalQuit {
			return messageCommand(QuitIntent{})
		}
		return messageCommand(DisconnectIntent{})
	}
	model.previousFocus = model.focus
	model.modal = modalState{Kind: kind}
	model.status = "Dirty or running tabs require confirmation."
	model.rebuildHitboxes()
	return nil
}

func (model Model) hasHazardousTabs() bool {
	for _, tab := range model.tabs {
		if tab.Envelope.Dirty || tab.Envelope.Lifecycle == TabRunning {
			return true
		}
	}
	return false
}

func (model *Model) updateModalKey(key string) tea.Cmd {
	switch key {
	case "esc":
		model.modal = modalState{}
		model.focus = model.previousFocus
		model.status = "Action cancelled; tabs were left unchanged."
	case "tab", "shift+tab", "left", "right", "h", "l":
		model.modal.Destructive = !model.modal.Destructive
	case "enter":
		if !model.modal.Destructive {
			model.modal = modalState{}
			model.focus = model.previousFocus
			model.status = "Action cancelled; tabs were left unchanged."
			break
		}
		modal := model.modal
		model.modal = modalState{}
		switch modal.Kind {
		case modalCloseTab:
			index := -1
			for candidate := range model.tabs {
				if model.tabs[candidate].Envelope.ID == modal.Tab {
					index = candidate
					break
				}
			}
			if index >= 0 {
				operation := model.tabs[index].Envelope.ActiveRequest.Operation
				model.closeTabAt(index)
				if operation != "" {
					return messageCommand(CancelIntent{Operations: []core.OperationID{operation}})
				}
			}
		case modalDisconnect:
			return messageCommand(DisconnectIntent{})
		case modalQuit:
			return messageCommand(QuitIntent{})
		}
	}
	model.rebuildHitboxes()
	return nil
}

func (model *Model) moveTab(delta int) {
	if len(model.tabs) == 0 {
		return
	}
	index := model.activeTabIndex()
	if index < 0 {
		index = 0
	} else {
		index = (index + delta + len(model.tabs)) % len(model.tabs)
	}
	model.activeTab = model.tabs[index].Envelope.ID
	if model.focus == FocusContent {
		model.focus = model.tabs[index].Envelope.LastFocus
	}
	model.rememberFocus()
	model.status = "Active tab: " + model.tabs[index].Envelope.Title + "."
}

func (model *Model) rememberFocus() {
	index := model.activeTabIndex()
	if index >= 0 && model.focus == FocusContent {
		model.tabs[index].Envelope.LastFocus = model.focus
	}
}

func (model *Model) moveFocus(delta int) {
	focuses := []Focus{FocusNavigator, FocusTabs}
	if model.activeTab != "" {
		focuses = append(focuses, FocusContent)
	}
	index := 0
	for candidate, focus := range focuses {
		if focus == model.focus {
			index = candidate
			break
		}
	}
	model.rememberFocus()
	model.focus = focuses[(index+delta+len(focuses))%len(focuses)]
}

func (model *Model) moveTree(delta int) {
	count := len(model.visibleItems())
	if count == 0 {
		model.selectedTree = 0
		return
	}
	model.selectedTree = (model.selectedTree + delta + count) % count
	model.ensureTreeVisible()
	model.rebuildHitboxes()
}

func (model Model) visibleItems() []treeItem {
	items := make([]treeItem, 0, len(model.schemas)*2)
	for schemaIndex, schema := range model.schemas {
		items = append(items, treeItem{Kind: treeSchemaItem, Schema: schemaIndex})
		if schema.Expanded {
			for relationIndex := range schema.Relations {
				items = append(items, treeItem{Kind: treeRelationItem, Schema: schemaIndex, Relation: relationIndex})
			}
		}
	}
	return items
}

func (model Model) selectedItem() (treeItem, bool) {
	items := model.visibleItems()
	if len(items) == 0 || model.selectedTree < 0 || model.selectedTree >= len(items) {
		return treeItem{}, false
	}
	return items[model.selectedTree], true
}

func (model *Model) ensureTreeVisible() {
	items := model.visibleItems()
	if len(items) == 0 {
		model.selectedTree = 0
		model.treeOffset = 0
		return
	}
	if model.selectedTree >= len(items) {
		model.selectedTree = len(items) - 1
	}
	visible := max(1, model.height-4)
	if model.selectedTree < model.treeOffset {
		model.treeOffset = model.selectedTree
	}
	if model.selectedTree >= model.treeOffset+visible {
		model.treeOffset = model.selectedTree - visible + 1
	}
	maxOffset := max(0, len(items)-visible)
	if model.treeOffset > maxOffset {
		model.treeOffset = maxOffset
	}
}

func (model Model) activeTabIndex() int {
	for index, tab := range model.tabs {
		if tab.Envelope.ID == model.activeTab {
			return index
		}
	}
	return -1
}

func (model Model) schemaIndex(name string) int {
	for index, schema := range model.schemas {
		if schema.Name == name {
			return index
		}
	}
	return -1
}

func (model *Model) newRequest() core.RequestID {
	request := model.nextRequest
	model.nextRequest++
	return request
}

func (model Model) meta(kind operationKind, request core.RequestID, schema string) core.RequestMeta {
	operation := fmt.Sprintf("workspace.%s.%d", kind, request)
	if schema != "" {
		operation = fmt.Sprintf("workspace.%s.%s.%d", kind, schema, request)
	}
	return core.RequestMeta{Workspace: model.id, Operation: core.OperationID(operation), Request: request}
}

func (model Model) matches(kind operationKind, actual core.RequestMeta, request core.RequestID, schema string) bool {
	return request != 0 && actual.Matches(model.meta(kind, request, schema))
}

func scheduleHealthCheck(workspace core.WorkspaceID, generation uint64) tea.Cmd {
	return tea.Tick(healthCheckInterval, func(time.Time) tea.Msg {
		return healthCheckDueMsg{Workspace: workspace, Generation: generation}
	})
}

func messageCommand(message tea.Msg) tea.Cmd {
	return func() tea.Msg { return message }
}
