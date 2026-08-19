// Package app owns the root Bubble Tea model and application lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/grid"
	"github.com/Jason-Wang1245/db-tui/internal/launcher"
	"github.com/Jason-Wang1245/db-tui/internal/profile"
	"github.com/Jason-Wang1245/db-tui/internal/ui"
	"github.com/Jason-Wang1245/db-tui/internal/workspace"
)

type ProfileService interface {
	Load(context.Context) (profile.Document, error)
	Draft(context.Context, profile.ID) (profile.Draft, error)
	Prepare(context.Context, profile.Draft) (profile.Prepared, error)
	Save(context.Context, profile.Draft) (profile.SaveResult, error)
	Commit(context.Context, profile.Prepared, bool) (profile.SaveResult, error)
	Delete(context.Context, profile.ID) error
}

type Dependencies struct {
	Diagnostics   *Diagnostics
	Profiles      ProfileService
	Connector     launcher.Connector
	Cancellations *CancellationRegistry
}

type surface string

const (
	surfaceLauncher  surface = "launcher"
	surfaceWorkspace surface = "workspace"
)

type Model struct {
	width          int
	height         int
	diagnostics    *Diagnostics
	theme          ui.Theme
	launcher       launcher.Model
	launcherReady  bool
	profiles       ProfileService
	connector      launcher.Connector
	cancellations  *CancellationRegistry
	surface        surface
	session        launcher.Session
	connected      launcher.Profile
	connection     launcher.ConnectionInfo
	warning        *core.Error
	workspace      workspace.Model
	workspaceReady bool
	catalog        workspace.CatalogReader
	browser        grid.TableBrowser
	tables         map[core.TabID]grid.Model
	nextWorkspace  uint64
}

type connectionSucceededMsg struct {
	profile launcher.Profile
	session launcher.Session
	info    launcher.ConnectionInfo
	warning *core.Error
	request core.RequestID
}

type reconnectionSucceededMsg struct {
	session launcher.Session
	info    launcher.ConnectionInfo
	meta    core.RequestMeta
}

func New(deps Dependencies) Model {
	diagnostics := deps.Diagnostics
	if diagnostics == nil {
		diagnostics = NewDiagnostics(256)
	}
	cancellations := deps.Cancellations
	if cancellations == nil {
		cancellations = NewCancellationRegistry()
	}
	model := Model{
		diagnostics:   diagnostics,
		theme:         ui.DefaultTheme(),
		profiles:      deps.Profiles,
		connector:     deps.Connector,
		cancellations: cancellations,
		surface:       surfaceLauncher,
		nextWorkspace: 1,
		tables:        make(map[core.TabID]grid.Model),
	}
	if deps.Profiles != nil && deps.Connector != nil {
		model.launcher = launcher.NewModel()
		model.launcherReady = true
	}
	return model
}

func (model Model) Init() tea.Cmd {
	if model.launcherReady {
		return model.launcher.Init()
	}
	return nil
}

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		if model.launcherReady {
			model.launcher.SetSize(message.Width, message.Height)
			model.layoutLauncherHitboxes()
		}
		if model.workspaceReady {
			model.workspace.SetSize(message.Width, message.Height)
			model.resizeTables()
		}
		return model, nil
	case launcher.LoadProfilesIntent:
		return model, model.loadProfiles(message)
	case launcher.LoadDraftIntent:
		return model, model.loadDraft(message)
	case launcher.SaveProfileIntent:
		return model, model.saveProfile(message)
	case launcher.DeleteProfileIntent:
		return model, model.deleteProfile(message)
	case launcher.TestConnectionIntent:
		return model, model.testConnection(message)
	case launcher.ConnectIntent:
		return model, model.connect(message)
	case launcher.ImportURLIntent:
		return model, model.importURL(message)
	case launcher.CancelIntent:
		return model, model.cancel(message)
	case workspace.LoadSchemasIntent:
		return model, model.loadSchemas(message)
	case workspace.LoadRelationsIntent:
		return model, model.loadRelations(message)
	case workspace.CheckConnectionIntent:
		return model, model.checkWorkspaceConnection(message)
	case workspace.ReconnectIntent:
		return model, model.reconnectWorkspace(message)
	case workspace.OpenTableIntent:
		return model.openTable(message)
	case workspace.CancelIntent:
		return model, model.cancelWorkspace(message)
	case workspace.DisconnectIntent:
		return model.disconnectWorkspace()
	case workspace.QuitIntent:
		return model, model.shutdownCommand()
	case grid.DescribeIntent:
		return model, model.describeTable(message)
	case grid.LoadPageIntent:
		return model, model.loadTablePage(message)
	case grid.CancelIntent:
		return model, model.cancelTable(message)
	case grid.RelationDescribedMsg:
		return model.updateTable(message.Meta.Tab, message)
	case grid.PageLoadedMsg:
		return model.updateTable(message.Meta.Tab, message)
	case grid.OperationFailedMsg:
		return model.updateTable(message.Meta.Tab, message)
	case connectionSucceededMsg:
		if !model.launcher.Expects(launcher.ActionConnect, message.request) {
			message.session.Close()
			return model, nil
		}
		catalog := catalogFromSession(message.session)
		if catalog == nil {
			message.session.Close()
			model.launcher.Update(launcher.OperationFailedMsg{
				Action: launcher.ActionConnect, Err: catalogUnavailableError(), Request: message.request,
			})
			return model, nil
		}
		model.launcher.Update(launcher.ConnectedMsg{Request: message.request})
		if model.session != nil {
			model.session.Close()
		}
		model.session = message.session
		model.connected = message.profile
		model.connection = message.info
		model.warning = message.warning
		model.surface = surfaceWorkspace
		model.catalog = catalog
		model.browser = browserFromSession(message.session)
		model.tables = make(map[core.TabID]grid.Model)
		workspaceID := core.WorkspaceID(fmt.Sprintf("workspace-%d", model.nextWorkspace))
		model.nextWorkspace++
		model.workspace = workspace.NewModel(
			workspaceID, message.profile.Name, message.info.Database, message.info.Server,
			message.info.ServerVersion, message.warning,
		)
		model.workspaceReady = true
		model.workspace.SetSize(model.width, model.height)
		model.diagnostics.Record(Diagnostic{Operation: "connect", Status: "success"})
		return model, model.workspace.Init()
	case reconnectionSucceededMsg:
		if !model.workspaceReady || !model.workspace.ExpectsReconnect(message.meta) {
			message.session.Close()
			return model, nil
		}
		catalog := catalogFromSession(message.session)
		if catalog == nil {
			message.session.Close()
			return model, model.workspace.Update(workspace.ReconnectFailed(message.meta, catalogUnavailableError()))
		}
		model.cancellations.CancelAll()
		if model.session != nil {
			model.session.Close()
		}
		model.session = message.session
		model.catalog = catalog
		model.browser = browserFromSession(message.session)
		model.connection = message.info
		model.diagnostics.Record(Diagnostic{Operation: "reconnect", Status: "success"})
		return model, model.workspace.Update(workspace.ReconnectedMsg{Meta: message.meta})
	case tea.KeyPressMsg:
		if message.Keystroke() == "ctrl+c" {
			if model.surface == surfaceWorkspace && model.workspaceReady {
				if model.workspace.RoutesToContent(message) {
					if table, ok := model.activeTable(); ok && table.Busy() {
						return model.routeToTable(message)
					}
				}
				return model.updateWorkspace(message)
			}
			return model, model.shutdownCommand()
		}
		if model.surface == surfaceWorkspace {
			if model.workspace.RoutesToContent(message) {
				return model.routeToTable(message)
			}
			return model.updateWorkspace(message)
		}
		if model.launcherReady && message.Keystroke() == "q" && model.launcher.CanQuit() {
			return model, model.shutdownCommand()
		}
	}
	if model.workspaceReady && model.surface == surfaceWorkspace {
		if model.workspace.RoutesToContent(message) {
			return model.routeToTable(message)
		}
		return model.updateWorkspace(message)
	}
	if model.launcherReady && model.surface == surfaceLauncher {
		return model, model.launcher.Update(message)
	}
	return model, nil
}

func (model *Model) layoutLauncherHitboxes() {
	hitboxes := []launcher.Hitbox{
		{Rect: ui.Rect{X: 0, Y: 3, Width: 5, Height: 1}, Action: launcher.MouseNew},
		{Rect: ui.Rect{X: 7, Y: 3, Width: 6, Height: 1}, Action: launcher.MouseEdit},
		{Rect: ui.Rect{X: 15, Y: 3, Width: 6, Height: 1}, Action: launcher.MouseTest},
		{Rect: ui.Rect{X: 23, Y: 3, Width: 9, Height: 1}, Action: launcher.MouseConnect},
		{Rect: ui.Rect{X: 34, Y: 3, Width: 8, Height: 1}, Action: launcher.MouseDelete},
	}
	for index := 0; index < max(0, model.height-9); index++ {
		hitboxes = append(hitboxes, launcher.Hitbox{
			Rect: ui.Rect{X: 0, Y: 7 + index, Width: model.width, Height: 1}, Action: launcher.MouseSelect, Profile: index,
		})
	}
	model.launcher.SetHitboxes(hitboxes)
}

func (model Model) loadProfiles(intent launcher.LoadProfilesIntent) tea.Cmd {
	service := model.profiles
	return model.run(launcher.ActionLoad, intent.Request, func(ctx context.Context) tea.Msg {
		document, err := service.Load(ctx)
		if err != nil {
			return operationFailure(launcher.ActionLoad, intent.Request, err)
		}
		profiles := make([]launcher.Profile, 0, len(document.Profiles))
		for _, saved := range document.Profiles {
			profiles = append(profiles, toLauncherProfile(saved))
		}
		return launcher.ProfilesLoadedMsg{Profiles: profiles, Request: intent.Request}
	})
}

func (model Model) loadDraft(intent launcher.LoadDraftIntent) tea.Cmd {
	service := model.profiles
	return model.run(launcher.ActionEdit, intent.Request, func(ctx context.Context) tea.Msg {
		draft, err := service.Draft(ctx, profile.ID(intent.ID))
		if err != nil {
			return operationFailure(launcher.ActionEdit, intent.Request, err)
		}
		return launcher.DraftLoadedMsg{Draft: toLauncherDraft(draft), Then: intent.Then, Request: intent.Request}
	})
}

func (model Model) saveProfile(intent launcher.SaveProfileIntent) tea.Cmd {
	service := model.profiles
	draft := toProfileDraft(intent.Draft)
	return model.run(launcher.ActionSave, intent.Request, func(ctx context.Context) tea.Msg {
		result, err := service.Save(ctx, draft)
		if err != nil {
			return operationFailure(launcher.ActionSave, intent.Request, err)
		}
		return launcher.ProfileSavedMsg{Profile: toLauncherProfile(result.Profile), Warning: result.Warning, Request: intent.Request}
	})
}

func (model Model) deleteProfile(intent launcher.DeleteProfileIntent) tea.Cmd {
	service := model.profiles
	return model.run(launcher.ActionDelete, intent.Request, func(ctx context.Context) tea.Msg {
		if err := service.Delete(ctx, profile.ID(intent.ID)); err != nil {
			return operationFailure(launcher.ActionDelete, intent.Request, err)
		}
		return launcher.ProfileDeletedMsg{Request: intent.Request}
	})
}

func (model Model) testConnection(intent launcher.TestConnectionIntent) tea.Cmd {
	service := model.profiles
	connector := model.connector
	draft := toProfileDraft(intent.Draft)
	return model.run(launcher.ActionTest, intent.Request, func(ctx context.Context) tea.Msg {
		prepared, err := service.Prepare(ctx, draft)
		if err != nil {
			return operationFailure(launcher.ActionTest, intent.Request, err)
		}
		if prepared.CredentialSource == profile.CredentialNone && !draft.ReplacePassword {
			return launcher.PasswordRequiredMsg{Draft: toLauncherDraft(draft), Then: launcher.ActionTest, Request: intent.Request}
		}
		info, err := connector.Test(ctx, targetFromProfile(prepared.Profile), launcher.Credential{Password: prepared.Password})
		if err != nil {
			return operationFailure(launcher.ActionTest, intent.Request, err)
		}
		return launcher.ConnectionTestedMsg{Info: info, Warning: prepared.Warning, Request: intent.Request}
	})
}

func (model Model) connect(intent launcher.ConnectIntent) tea.Cmd {
	service := model.profiles
	connector := model.connector
	draft := toProfileDraft(intent.Draft)
	return model.run(launcher.ActionConnect, intent.Request, func(ctx context.Context) tea.Msg {
		prepared, err := service.Prepare(ctx, draft)
		if err != nil {
			return operationFailure(launcher.ActionConnect, intent.Request, err)
		}
		if prepared.CredentialSource == profile.CredentialNone && !draft.ReplacePassword {
			return launcher.PasswordRequiredMsg{Draft: toLauncherDraft(draft), Then: launcher.ActionConnect, Request: intent.Request}
		}
		session, info, err := connector.Connect(ctx, targetFromProfile(prepared.Profile), launcher.Credential{Password: prepared.Password})
		if err != nil {
			return operationFailure(launcher.ActionConnect, intent.Request, err)
		}
		result, err := service.Commit(ctx, prepared, true)
		if err != nil {
			session.Close()
			return operationFailure(launcher.ActionConnect, intent.Request, err)
		}
		warning := result.Warning
		if warning == nil {
			warning = prepared.Warning
		}
		return connectionSucceededMsg{
			profile: toLauncherProfile(result.Profile), session: session, info: info,
			warning: warning, request: intent.Request,
		}
	})
}

func (model Model) loadSchemas(intent workspace.LoadSchemasIntent) tea.Cmd {
	catalog := model.catalog
	return model.runWorkspace(intent.Meta, func(ctx context.Context) tea.Msg {
		if catalog == nil {
			return workspace.SchemasFailed(intent.Meta, catalogUnavailableError())
		}
		schemas, err := catalog.Schemas(ctx)
		if err != nil {
			return workspace.SchemasFailed(intent.Meta, workspaceOperationError("load schemas", err))
		}
		return workspace.SchemasLoadedMsg{Schemas: schemas, Meta: intent.Meta}
	})
}

func (model Model) loadRelations(intent workspace.LoadRelationsIntent) tea.Cmd {
	catalog := model.catalog
	return model.runWorkspace(intent.Meta, func(ctx context.Context) tea.Msg {
		if catalog == nil {
			return workspace.RelationsFailed(intent.Schema, intent.Meta, catalogUnavailableError())
		}
		relations, err := catalog.Relations(ctx, intent.Schema)
		if err != nil {
			return workspace.RelationsFailed(intent.Schema, intent.Meta, workspaceOperationError("load relations", err))
		}
		return workspace.RelationsLoadedMsg{Schema: intent.Schema, Relations: relations, Meta: intent.Meta}
	})
}

func (model Model) openTable(intent workspace.OpenTableIntent) (tea.Model, tea.Cmd) {
	if !model.workspaceReady || intent.Workspace != model.workspace.ID() {
		return model, nil
	}
	if _, exists := model.tables[intent.Tab]; exists {
		return model, nil
	}
	table := grid.NewModel(intent.Workspace, intent.Tab, grid.RelationID{
		Schema: intent.Relation.Schema,
		Name:   intent.Relation.Name,
	})
	rect := model.workspace.ContentRect()
	table.SetSize(rect.Width, rect.Height)
	command := table.Init()
	model.tables[intent.Tab] = table
	model.syncTableState(intent.Tab, table)
	return model, command
}

func (model Model) describeTable(intent grid.DescribeIntent) tea.Cmd {
	browser := model.browser
	return model.runWorkspace(intent.Meta, func(ctx context.Context) tea.Msg {
		if browser == nil {
			return grid.DescribeFailed(intent.Meta, browserUnavailableError())
		}
		relation, err := browser.Describe(ctx, intent.Relation)
		if err != nil {
			return grid.DescribeFailed(intent.Meta, workspaceOperationError("describe relation", err))
		}
		return grid.RelationDescribedMsg{Relation: relation, Meta: intent.Meta}
	})
}

func (model Model) loadTablePage(intent grid.LoadPageIntent) tea.Cmd {
	browser := model.browser
	return model.runWorkspace(intent.Meta, func(ctx context.Context) tea.Msg {
		if browser == nil {
			return grid.PageFailed(intent.Meta, browserUnavailableError())
		}
		page, err := browser.FetchPage(ctx, intent.Relation, intent.Page)
		if err != nil {
			return grid.PageFailed(intent.Meta, workspaceOperationError("fetch table page", err))
		}
		return grid.PageLoadedMsg{Page: page, Meta: intent.Meta}
	})
}

func (model Model) cancelTable(intent grid.CancelIntent) tea.Cmd {
	registry := model.cancellations
	return func() tea.Msg {
		registry.Cancel(intent.Operation)
		return nil
	}
}

func (model Model) updateTable(tab core.TabID, message tea.Msg) (tea.Model, tea.Cmd) {
	table, ok := model.tables[tab]
	if !ok {
		return model, nil
	}
	command := table.Update(message)
	model.tables[tab] = table
	model.syncTableState(tab, table)
	return model, command
}

func (model Model) routeToTable(message tea.Msg) (tea.Model, tea.Cmd) {
	tab, _, ok := model.workspace.ActiveTable()
	if !ok {
		return model.updateWorkspace(message)
	}
	table, ok := model.tables[tab]
	if !ok {
		return model.updateWorkspace(message)
	}
	var workspaceCommand tea.Cmd
	gridMessage := message
	switch message := message.(type) {
	case tea.MouseClickMsg:
		workspaceCommand = model.workspace.Update(message)
		mouse := message.Mouse()
		rect := model.workspace.ContentRect()
		mouse.X -= rect.X
		mouse.Y -= rect.Y
		gridMessage = tea.MouseClickMsg(mouse)
	case tea.MouseWheelMsg:
		mouse := message.Mouse()
		rect := model.workspace.ContentRect()
		mouse.X -= rect.X
		mouse.Y -= rect.Y
		gridMessage = tea.MouseWheelMsg(mouse)
	}
	tableCommand := table.Update(gridMessage)
	model.tables[tab] = table
	model.syncTableState(tab, table)
	return model, combineCommands(workspaceCommand, tableCommand)
}

func (model Model) updateWorkspace(message tea.Msg) (tea.Model, tea.Cmd) {
	command := model.workspace.Update(message)
	model.reconcileTables()
	return model, command
}

func (model *Model) resizeTables() {
	rect := model.workspace.ContentRect()
	for tab, table := range model.tables {
		table.SetSize(rect.Width, rect.Height)
		model.tables[tab] = table
	}
}

func (model Model) activeTable() (grid.Model, bool) {
	tab, _, ok := model.workspace.ActiveTable()
	if !ok {
		return grid.Model{}, false
	}
	table, ok := model.tables[tab]
	return table, ok
}

func (model *Model) syncTableState(tab core.TabID, table grid.Model) {
	lifecycle := workspace.TabIdle
	switch table.Lifecycle() {
	case grid.LifecycleRunning:
		lifecycle = workspace.TabRunning
	case grid.LifecycleFailed:
		lifecycle = workspace.TabFailed
	}
	model.workspace.Update(workspace.TabStateChangedMsg{
		Tab: tab, Lifecycle: lifecycle, Request: table.ActiveMeta(),
	})
}

func (model *Model) reconcileTables() {
	open := make(map[core.TabID]bool)
	for _, tab := range model.workspace.Tabs() {
		if tab.Envelope.Kind == workspace.TabTable {
			open[tab.Envelope.ID] = true
		}
	}
	for tab := range model.tables {
		if !open[tab] {
			delete(model.tables, tab)
		}
	}
}

func combineCommands(commands ...tea.Cmd) tea.Cmd {
	filtered := make([]tea.Cmd, 0, len(commands))
	for _, command := range commands {
		if command != nil {
			filtered = append(filtered, command)
		}
	}
	switch len(filtered) {
	case 0:
		return nil
	case 1:
		return filtered[0]
	default:
		return tea.Batch(filtered...)
	}
}

func browserFromSession(session launcher.Session) grid.TableBrowser {
	if browser, ok := session.(grid.TableBrowser); ok {
		return browser
	}
	return nil
}

func browserUnavailableError() *core.Error {
	return core.NewError(
		"browse table", core.ErrorInternal,
		"This connection does not expose PostgreSQL table browsing.", false, nil,
	)
}

func (model Model) checkWorkspaceConnection(intent workspace.CheckConnectionIntent) tea.Cmd {
	session := model.session
	return model.runWorkspace(intent.Meta, func(ctx context.Context) tea.Msg {
		if session == nil {
			return workspace.HealthCheckFailed(intent.Meta, core.NewError(
				"check connection", core.ErrorNetwork, "The PostgreSQL session is no longer available.", true, nil,
			))
		}
		if err := session.Ping(ctx); err != nil {
			return workspace.HealthCheckFailed(intent.Meta, workspaceOperationError("check connection", err))
		}
		return workspace.ConnectionCheckedMsg{Meta: intent.Meta}
	})
}

func (model Model) reconnectWorkspace(intent workspace.ReconnectIntent) tea.Cmd {
	profiles := model.profiles
	connector := model.connector
	connected := model.connected
	return model.runWorkspace(intent.Meta, func(ctx context.Context) tea.Msg {
		if profiles == nil || connector == nil || connected.ID == "" {
			return workspace.ReconnectFailed(intent.Meta, core.NewError(
				"reconnect", core.ErrorInternal, "The saved connection profile is unavailable.", false, nil,
			))
		}
		draft, err := profiles.Draft(ctx, profile.ID(connected.ID))
		if err != nil {
			return workspace.ReconnectFailed(intent.Meta, workspaceOperationError("reconnect", err))
		}
		prepared, err := profiles.Prepare(ctx, draft)
		if err != nil {
			return workspace.ReconnectFailed(intent.Meta, workspaceOperationError("reconnect", err))
		}
		if prepared.CredentialSource == profile.CredentialNone && !draft.ReplacePassword {
			return workspace.ReconnectFailed(intent.Meta, core.NewError(
				"reconnect", core.ErrorAuthentication,
				"No password is available for automatic reconnect. Disconnect and enter it from the launcher.", false, nil,
			))
		}
		session, info, err := connector.Connect(
			ctx, targetFromProfile(prepared.Profile), launcher.Credential{Password: prepared.Password},
		)
		if err != nil {
			return workspace.ReconnectFailed(intent.Meta, workspaceOperationError("reconnect", err))
		}
		return reconnectionSucceededMsg{session: session, info: info, meta: intent.Meta}
	})
}

func (model Model) runWorkspace(meta core.RequestMeta, work func(context.Context) tea.Msg) tea.Cmd {
	registry := model.cancellations
	ctx, cancel := context.WithCancel(context.Background())
	registry.Register(meta.Operation, cancel)
	return func() tea.Msg {
		defer registry.Forget(meta.Operation)
		defer cancel()
		return work(ctx)
	}
}

func (model Model) cancelWorkspace(intent workspace.CancelIntent) tea.Cmd {
	registry := model.cancellations
	operations := append([]core.OperationID(nil), intent.Operations...)
	return func() tea.Msg {
		for _, operation := range operations {
			registry.Cancel(operation)
		}
		return nil
	}
}

func (model Model) disconnectWorkspace() (tea.Model, tea.Cmd) {
	registry := model.cancellations
	session := model.session
	model.session = nil
	model.catalog = nil
	model.browser = nil
	model.tables = make(map[core.TabID]grid.Model)
	model.workspaceReady = false
	model.surface = surfaceLauncher
	model.warning = nil
	model.diagnostics.Record(Diagnostic{Operation: "disconnect", Status: "success"})
	return model, tea.Sequence(
		func() tea.Msg {
			registry.CancelAll()
			if session != nil {
				session.Close()
			}
			return nil
		},
		model.launcher.Reload(),
	)
}

func catalogFromSession(session launcher.Session) workspace.CatalogReader {
	if catalog, ok := session.(workspace.CatalogReader); ok {
		return catalog
	}
	return nil
}

func catalogUnavailableError() *core.Error {
	return core.NewError(
		"load catalog", core.ErrorInternal,
		"This connection does not expose PostgreSQL catalog access.", false, nil,
	)
}

func workspaceOperationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var structured *core.Error
	if errors.As(err, &structured) {
		return structured
	}
	if errors.Is(err, context.Canceled) {
		return core.NewError(operation, core.ErrorCancellation, "Operation cancelled. You can run it again.", true, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return core.NewError(operation, core.ErrorTimeout, "PostgreSQL did not respond in time. You can try again.", true, err)
	}
	return core.NewError(operation, core.ErrorInternal, "The workspace operation failed.", true, err)
}

func (model Model) importURL(intent launcher.ImportURLIntent) tea.Cmd {
	raw := intent.Raw
	return model.run(launcher.ActionEdit, intent.Request, func(ctx context.Context) tea.Msg {
		if err := ctx.Err(); err != nil {
			return operationFailure(launcher.ActionEdit, intent.Request, err)
		}
		result, err := profile.ImportURL(raw)
		raw = ""
		if err != nil {
			return operationFailure(launcher.ActionEdit, intent.Request, err)
		}
		return launcher.URLImportedMsg{Draft: toLauncherDraft(result.Draft), Request: intent.Request}
	})
}

func (model Model) cancel(intent launcher.CancelIntent) tea.Cmd {
	registry := model.cancellations
	return func() tea.Msg {
		registry.Cancel(operationID(intent.Action, intent.Request))
		return nil
	}
}

func (model Model) run(action launcher.Action, request core.RequestID, work func(context.Context) tea.Msg) tea.Cmd {
	registry := model.cancellations
	ctx, cancel := context.WithCancel(context.Background())
	id := operationID(action, request)
	registry.Register(id, cancel)
	return func() tea.Msg {
		defer registry.Forget(id)
		defer cancel()
		return work(ctx)
	}
}

func (model Model) shutdownCommand() tea.Cmd {
	registry := model.cancellations
	session := model.session
	return tea.Sequence(
		func() tea.Msg {
			registry.CancelAll()
			if session != nil {
				session.Close()
			}
			return nil
		},
		tea.Quit,
	)
}

func closeSessionCommand(session launcher.Session) tea.Cmd {
	return func() tea.Msg {
		if session != nil {
			session.Close()
		}
		return nil
	}
}

func operationID(action launcher.Action, request core.RequestID) core.OperationID {
	return core.OperationID(fmt.Sprintf("launcher.%s.%d", action, request))
}

func operationFailure(action launcher.Action, request core.RequestID, err error) launcher.OperationFailedMsg {
	if errors.Is(err, context.Canceled) {
		err = core.NewError(string(action), core.ErrorCancellation, "Operation cancelled. You can run it again.", true, err)
	} else if errors.Is(err, context.DeadlineExceeded) {
		err = core.NewError(string(action), core.ErrorTimeout, "Operation timed out. Check the connection and try again.", true, err)
	}
	return launcher.OperationFailedMsg{Action: action, Err: err, Request: request}
}

func toLauncherProfile(saved profile.Profile) launcher.Profile {
	return launcher.Profile{
		ID: string(saved.ID), Name: saved.Name, Host: saved.Host, Port: saved.Port,
		Database: saved.Database, User: saved.User, SSLMode: saved.SSLMode,
		Advanced: cloneParameters(saved.AdvancedParameters), SavePassword: saved.SavePassword,
		LastUsedAt: saved.LastUsedAt,
	}
}

func toLauncherDraft(draft profile.Draft) launcher.Draft {
	parameters := make([]launcher.Parameter, 0, len(draft.AdvancedParameters))
	for _, parameter := range draft.AdvancedParameters {
		parameters = append(parameters, launcher.Parameter{Name: parameter.Name, Value: parameter.Value})
	}
	return launcher.Draft{
		ID: string(draft.ID), Name: draft.Name, Host: draft.Host, Port: draft.Port,
		Database: draft.Database, User: draft.User, Password: draft.Password,
		ReplacePassword: draft.ReplacePassword, HasStoredPassword: draft.HasStoredPassword,
		SavePassword: draft.SavePassword, SSLMode: draft.SSLMode, AdvancedParameters: parameters,
	}
}

func toProfileDraft(draft launcher.Draft) profile.Draft {
	parameters := make([]profile.Parameter, 0, len(draft.AdvancedParameters))
	for _, parameter := range draft.AdvancedParameters {
		parameters = append(parameters, profile.Parameter{Name: parameter.Name, Value: parameter.Value})
	}
	return profile.Draft{
		ID: profile.ID(draft.ID), Name: draft.Name, Host: draft.Host, Port: draft.Port,
		Database: draft.Database, User: draft.User, Password: draft.Password,
		ReplacePassword: draft.ReplacePassword, HasStoredPassword: draft.HasStoredPassword,
		SavePassword: draft.SavePassword, SSLMode: draft.SSLMode, AdvancedParameters: parameters,
	}
}

func targetFromProfile(saved profile.Profile) launcher.ConnectionTarget {
	return launcher.ConnectionTarget{
		Host: saved.Host, Port: saved.Port, Database: saved.Database, User: saved.User,
		SSLMode: saved.SSLMode, AdvancedParameters: cloneParameters(saved.AdvancedParameters),
	}
}

func cloneParameters(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for name, value := range source {
		result[name] = value
	}
	return result
}

func (model Model) View() tea.View {
	content := model.bootstrapView()
	if model.width > 0 && model.height > 0 && (model.width < 48 || model.height < 12) {
		preserved := "Launcher state is preserved."
		if model.surface == surfaceWorkspace && model.workspaceReady {
			preserved = fmt.Sprintf("Connected to %s / %s; workspace state is preserved.", model.connected.Name, model.connection.Database)
		}
		content = strings.Join([]string{
			model.theme.Title.Render("db-tui"), "",
			"Terminal is too small for safe editing.",
			fmt.Sprintf("Need at least 48×12; current size is %d×%d.", model.width, model.height),
			"Resize to continue. " + preserved, "",
			model.theme.Muted.Render("Ctrl+C quit"),
		}, "\n")
	} else if model.launcherReady {
		if model.surface == surfaceWorkspace {
			content = model.workspaceView()
		} else {
			content = model.launcher.View()
		}
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.ReportFocus = true
	return view
}

func (model Model) bootstrapView() string {
	lines := []string{
		model.theme.Title.Render("db-tui"), "",
		"PostgreSQL connection management is the next implementation slice.", "",
		model.theme.Muted.Render("q quit • ? help"),
	}
	if model.width > 0 && model.height > 0 {
		lines = append(lines, "", model.theme.Muted.Render(fmt.Sprintf("%d×%d", model.width, model.height)))
	}
	return strings.Join(lines, "\n")
}

func (model Model) workspaceView() string {
	if !model.workspaceReady {
		return model.bootstrapView()
	}
	content := workspace.ContentView{}
	if table, ok := model.activeTable(); ok {
		view := table.View(model.theme)
		content = workspace.ContentView{Lines: view.Lines, Hints: view.Hints, Status: view.Status}
	}
	return model.workspace.ViewWithContent(model.theme, content)
}
