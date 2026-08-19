package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/grid"
	"github.com/Jason-Wang1245/db-tui/internal/launcher"
	"github.com/Jason-Wang1245/db-tui/internal/profile"
	"github.com/Jason-Wang1245/db-tui/internal/sqltab"
	"github.com/Jason-Wang1245/db-tui/internal/workspace"
)

type fakeProfileService struct {
	document       profile.Document
	draft          profile.Draft
	prepared       profile.Prepared
	commitCalls    int
	markLastUsed   bool
	deleteCalls    int
	saveResult     profile.SaveResult
	operationError error
}

func (service *fakeProfileService) Load(context.Context) (profile.Document, error) {
	return service.document, service.operationError
}

func (service *fakeProfileService) Draft(context.Context, profile.ID) (profile.Draft, error) {
	return service.draft, service.operationError
}

func (service *fakeProfileService) Prepare(context.Context, profile.Draft) (profile.Prepared, error) {
	return service.prepared, service.operationError
}

func (service *fakeProfileService) Save(context.Context, profile.Draft) (profile.SaveResult, error) {
	return service.saveResult, service.operationError
}

func (service *fakeProfileService) Commit(_ context.Context, prepared profile.Prepared, markLastUsed bool) (profile.SaveResult, error) {
	service.commitCalls++
	service.markLastUsed = markLastUsed
	return profile.SaveResult{Profile: prepared.Profile}, service.operationError
}

func (service *fakeProfileService) Delete(context.Context, profile.ID) error {
	service.deleteCalls++
	return service.operationError
}

type fakeConnector struct {
	session *fakeSession
	info    launcher.ConnectionInfo
	err     error
}

func (connector *fakeConnector) Test(context.Context, launcher.ConnectionTarget, launcher.Credential) (launcher.ConnectionInfo, error) {
	return connector.info, connector.err
}

func (connector *fakeConnector) Connect(context.Context, launcher.ConnectionTarget, launcher.Credential) (launcher.Session, launcher.ConnectionInfo, error) {
	if connector.err != nil {
		return nil, launcher.ConnectionInfo{}, connector.err
	}
	return connector.session, connector.info, nil
}

type fakeSession struct {
	closes      int
	pingError   error
	schemas     []workspace.Schema
	relations   map[string][]workspace.Relation
	sqlResult   sqltab.RunResult
	sqlError    error
	sqlRuns     []sqltab.RunRequest
	applyResult grid.ApplyResult
	applyError  error
	applyRuns   []grid.ApplyRequest
}

func (session *fakeSession) Ping(context.Context) error { return session.pingError }
func (session *fakeSession) Close()                     { session.closes++ }
func (session *fakeSession) Schemas(context.Context) ([]workspace.Schema, error) {
	if session.schemas == nil {
		return []workspace.Schema{{Name: "public"}}, nil
	}
	return session.schemas, nil
}
func (session *fakeSession) Relations(_ context.Context, schema string) ([]workspace.Relation, error) {
	if session.relations == nil {
		return []workspace.Relation{{Schema: schema, Name: "users", Kind: workspace.RelationTable, CanSelect: true}}, nil
	}
	return session.relations[schema], nil
}

func (*fakeSession) Describe(_ context.Context, relation grid.RelationID) (grid.Relation, error) {
	return grid.Relation{
		ID: relation, Kind: grid.RelationTable, Identity: []string{"id"}, IdentityPrimary: true,
		CanSelect: true, CanInsert: true, CanUpdate: true, CanDelete: true, HasXMin: true,
		Columns: []grid.Column{
			{Name: "id", DataType: "bigint", TypeOID: 20, CanSelect: true, CanInsert: true, CanUpdate: true, Sortable: true, IdentityPart: true},
			{Name: "email", DataType: "text", TypeOID: 25, CanSelect: true, CanInsert: true, CanUpdate: true, Sortable: true},
		},
	}, nil
}

func (*fakeSession) FetchPage(_ context.Context, _ grid.Relation, request grid.PageRequest) (grid.Page, error) {
	return grid.Page{Rows: []grid.Row{{
		Identity: map[string]any{"id": int64(1)}, XMin: 7,
		Cells: []grid.Cell{{Raw: int64(1), Display: "1", Edit: "1"}, {Raw: "a@example.com", Display: "a@example.com", Edit: "a@example.com"}},
	}}}, nil
}

func (session *fakeSession) FetchCurrentRow(ctx context.Context, relation grid.RelationID, _ map[string]any) (grid.Row, error) {
	described, err := session.Describe(ctx, relation)
	if err != nil {
		return grid.Row{}, err
	}
	page, err := session.FetchPage(ctx, described, grid.PageRequest{Relation: relation})
	if err != nil {
		return grid.Row{}, err
	}
	return page.Rows[0], nil
}

func (session *fakeSession) Execute(_ context.Context, request sqltab.RunRequest) (sqltab.RunResult, error) {
	session.sqlRuns = append(session.sqlRuns, request)
	if session.sqlError != nil {
		return session.sqlResult, session.sqlError
	}
	if len(session.sqlResult.Outputs) > 0 {
		return session.sqlResult, nil
	}
	return sqltab.RunResult{Outputs: []sqltab.Output{{
		Kind:    sqltab.OutputRows,
		Columns: []sqltab.Column{{Name: "answer", DataType: "integer", TypeOID: 23}},
		Rows: [][]sqltab.Cell{
			{{Raw: int32(42), Display: "42", Full: "42"}},
		},
	}}}, nil
}

func (session *fakeSession) Apply(_ context.Context, request grid.ApplyRequest) (grid.ApplyResult, error) {
	session.applyRuns = append(session.applyRuns, request)
	if session.applyError != nil {
		return grid.ApplyResult{}, session.applyError
	}
	if session.applyResult != (grid.ApplyResult{}) {
		return session.applyResult, nil
	}
	result := grid.ApplyResult{}
	for _, mutation := range request.Mutations {
		switch mutation.Kind {
		case grid.MutationInsert:
			result.Inserted++
		case grid.MutationUpdate:
			result.Updated++
		case grid.MutationDelete:
			result.Deleted++
		}
	}
	return result, nil
}

func connectionFixture() (*fakeProfileService, *fakeConnector) {
	saved := profile.Profile{
		ID: "profile-1", Name: "Local", Host: "localhost", Port: 5432,
		Database: "app", User: "developer", SSLMode: "disable", SavePassword: true,
	}
	draft := profile.DraftFromProfile(saved, true)
	prepared := profile.Prepared{
		Profile: saved, Password: "secret", CredentialSource: profile.CredentialKeychain,
	}
	service := &fakeProfileService{
		document: profile.Document{Version: profile.CurrentDocumentVersion, Profiles: []profile.Profile{saved}},
		draft:    draft, prepared: prepared,
	}
	connector := &fakeConnector{
		session: &fakeSession{},
		info: launcher.ConnectionInfo{
			Server: "localhost:5432", Database: "app", ServerVersion: "18.1", Latency: 12 * time.Millisecond,
		},
	}
	return service, connector
}

func updateApp(t *testing.T, model Model, message tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, command := model.Update(message)
	return updated.(Model), command
}

func loadLauncher(t *testing.T, model Model) Model {
	t.Helper()
	intent := model.Init()()
	var command tea.Cmd
	model, command = updateApp(t, model, intent)
	model, _ = updateApp(t, model, command())
	return model
}

func beginQuickConnect(t *testing.T, model Model) (Model, tea.Cmd) {
	t.Helper()
	var command tea.Cmd
	model, command = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	model, command = updateApp(t, model, command())
	model, command = updateApp(t, model, command())
	model, command = updateApp(t, model, command())
	return model, command
}

func TestConnectCommitsOnlyAfterSuccessAndEntersWorkspace(t *testing.T) {
	service, connector := connectionFixture()
	model := loadLauncher(t, New(Dependencies{Profiles: service, Connector: connector}))
	model, command := beginQuickConnect(t, model)
	succeeded := command()
	if service.commitCalls != 1 || !service.markLastUsed {
		t.Fatalf("commit calls=%d markLastUsed=%v", service.commitCalls, service.markLastUsed)
	}
	model, _ = updateApp(t, model, succeeded)
	view := model.View().Content
	if !strings.Contains(view, "Local") || !strings.Contains(view, "PostgreSQL 18.1") {
		t.Fatalf("workspace view = %q", view)
	}
	if connector.session.closes != 0 {
		t.Fatal("successful session was closed")
	}
}

func TestFailedConnectStaysInLauncherAndDoesNotCommit(t *testing.T) {
	service, connector := connectionFixture()
	connector.err = core.NewError("connect", core.ErrorAuthentication, "PostgreSQL rejected the username or password.", false, nil)
	model := loadLauncher(t, New(Dependencies{Profiles: service, Connector: connector}))
	model, command := beginQuickConnect(t, model)
	model, _ = updateApp(t, model, command())
	if service.commitCalls != 0 {
		t.Fatal("failed connection was committed")
	}
	if view := model.View().Content; !strings.Contains(view, "rejected the username or password") || strings.Contains(view, "workspace shell") {
		t.Fatalf("launcher view = %q", view)
	}
}

func TestMissingPasswordOpensExplicitPrompt(t *testing.T) {
	service, connector := connectionFixture()
	service.prepared.CredentialSource = profile.CredentialNone
	service.prepared.Password = ""
	model := loadLauncher(t, New(Dependencies{Profiles: service, Connector: connector}))
	model, command := beginQuickConnect(t, model)
	model, _ = updateApp(t, model, command())
	if view := model.View().Content; !strings.Contains(view, "use an empty password explicitly") {
		t.Fatalf("password prompt view = %q", view)
	}
}

func TestUnsavedPasswordPromptDoesNotTurnGeneratedIDIntoSavedProfile(t *testing.T) {
	service, connector := connectionFixture()
	service.prepared.CredentialSource = profile.CredentialNone
	service.prepared.Password = ""
	service.prepared.Profile.ID = "generated-but-unsaved"
	model := New(Dependencies{Profiles: service, Connector: connector})
	intent := launcher.TestConnectionIntent{
		Draft: launcher.Draft{
			Name: "New", Host: "localhost", Port: "5432", Database: "app", User: "developer", SSLMode: "disable",
		},
		Request: 42,
	}
	message := model.testConnection(intent)()
	prompt, ok := message.(launcher.PasswordRequiredMsg)
	if !ok {
		t.Fatalf("message = %T", message)
	}
	if prompt.Draft.ID != "" {
		t.Fatalf("unsaved draft acquired ID %q", prompt.Draft.ID)
	}
}

func TestStaleConnectionSuccessIsClosedAndIgnored(t *testing.T) {
	service, connector := connectionFixture()
	model := loadLauncher(t, New(Dependencies{Profiles: service, Connector: connector}))
	stale := &fakeSession{}
	model, _ = updateApp(t, model, connectionSucceededMsg{
		profile: launcher.Profile{Name: "Stale"}, session: stale,
		request: 999,
	})
	if stale.closes != 1 {
		t.Fatal("stale session was not closed")
	}
	if strings.Contains(model.View().Content, "Stale") {
		t.Fatal("stale connection replaced launcher state")
	}
}

func TestConnectedWorkspaceLoadsCatalogOpensTableAndDisconnects(t *testing.T) {
	service, connector := connectionFixture()
	model := New(Dependencies{Profiles: service, Connector: connector})
	model, _ = updateApp(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model = loadLauncher(t, model)
	model, command := beginQuickConnect(t, model)
	model, command = updateApp(t, model, command())

	// The workspace init intent is executed by the app against the live session.
	model, command = updateApp(t, model, command())
	model, command = updateApp(t, model, command())
	if view := model.View().Content; !strings.Contains(view, "public") {
		t.Fatalf("catalog view = %q", view)
	}

	model, command = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeySpace})
	model, command = updateApp(t, model, command())
	model, command = updateApp(t, model, command())
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model, command = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	for step := 0; step < 5; step++ {
		if command == nil {
			t.Fatalf("table load stopped at step %d", step)
		}
		model, command = updateApp(t, model, command())
	}
	if view := model.View().Content; !strings.Contains(view, "public.users") || !strings.Contains(view, "a@example.com") {
		t.Fatalf("table grid view = %q", view)
	}

	// Grid editing captures ordinary text that would otherwise be a workspace
	// shortcut, then applies an immutable staged snapshot and reloads the page.
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 'e'})
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 'q', Text: "q"})
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	tabID := model.workspace.ActiveTab()
	if !model.tables[tabID].Dirty() || !model.workspace.Tabs()[0].Envelope.Dirty {
		t.Fatalf("staged grid state: table=%#v tabs=%#v", model.tables[tabID], model.workspace.Tabs())
	}
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 'a'})
	model, command = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	model, command = updateApp(t, model, command())
	model, command = updateApp(t, model, command())
	model, command = updateApp(t, model, command())
	model, command = updateApp(t, model, command())
	if len(connector.session.applyRuns) != 1 || len(connector.session.applyRuns[0].Mutations) != 1 ||
		connector.session.applyRuns[0].Mutations[0].Values["email"].Text != "a@example.comq" || model.tables[tabID].Dirty() {
		t.Fatalf("grid apply runs=%#v table=%#v", connector.session.applyRuns, model.tables[tabID])
	}

	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 'e'})
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 'x', Text: ".pending"})
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	oldSession := connector.session
	oldSession.pingError = core.NewError("ping", core.ErrorNetwork, "PostgreSQL closed the connection.", true, nil)
	model, command = updateApp(t, model, model.workspace.CheckConnectionNow())
	model, command = updateApp(t, model, command())
	model, _ = updateApp(t, model, command())
	newSession := &fakeSession{}
	connector.session = newSession
	model, command = updateApp(t, model, tea.KeyPressMsg{Code: 'r'})
	model, command = updateApp(t, model, command())
	model, _ = updateApp(t, model, command())
	if model.applier != newSession || !model.tables[tabID].Dirty() || !model.workspace.Tabs()[0].Envelope.Dirty {
		t.Fatalf("staged grid reconnect: applier=%T table=%#v tabs=%#v", model.applier, model.tables[tabID], model.workspace.Tabs())
	}
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 'u'})

	model, command = updateApp(t, model, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	model, command = updateApp(t, model, command())
	if model.surface != surfaceLauncher || model.workspaceReady || model.session != nil {
		t.Fatalf("disconnect state: surface=%s ready=%v session=%v", model.surface, model.workspaceReady, model.session)
	}
}

func TestConnectedWorkspaceRunsIndependentDirtySQLTabs(t *testing.T) {
	service, connector := connectionFixture()
	model := New(Dependencies{Profiles: service, Connector: connector})
	model, _ = updateApp(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model = loadLauncher(t, model)
	model, command := beginQuickConnect(t, model)
	model, _ = updateApp(t, model, command())

	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model, command = updateApp(t, model, tea.KeyPressMsg{Code: 'n', Text: "n"})
	model, _ = updateApp(t, model, command())
	firstTab := model.workspace.ActiveTab()
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 's', Text: "select 42"})
	state := model.sqlTabs[firstTab]
	if state.Buffer() != "select 42" || !model.workspace.Tabs()[0].Envelope.Dirty {
		t.Fatalf("SQL editor state=%q tabs=%#v", state.Buffer(), model.workspace.Tabs())
	}
	// A confirmation opened by a mouse/global action owns input even if the SQL
	// editor was still in text mode.
	model.workspace.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if model.sqlTabs[firstTab].Buffer() != "select 42" {
		t.Fatal("confirmation input leaked into the SQL editor")
	}
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyEsc})

	model, command = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyF5})
	model, command = updateApp(t, model, command())
	model, _ = updateApp(t, model, command())
	if len(connector.session.sqlRuns) != 1 || connector.session.sqlRuns[0].Snapshot != "select 42" {
		t.Fatalf("SQL runs = %#v", connector.session.sqlRuns)
	}
	if view := model.View().Content; !strings.Contains(view, "answer") || !strings.Contains(view, "42") {
		t.Fatalf("SQL result view = %q", view)
	}

	// The completed run focuses results. Esc returns to the tab strip, where a
	// second independent SQL tab can be opened.
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyEsc})
	model, command = updateApp(t, model, tea.KeyPressMsg{Code: 'n', Text: "n"})
	model, _ = updateApp(t, model, command())
	secondTab := model.workspace.ActiveTab()
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 'q', Text: "query two"})
	if secondTab == firstTab || model.sqlTabs[secondTab].Buffer() != "query two" {
		t.Fatalf("second SQL tab=%s state=%q", secondTab, model.sqlTabs[secondTab].Buffer())
	}
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyEsc})
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: '['})
	if model.workspace.ActiveTab() != firstTab || model.sqlTabs[firstTab].Buffer() != "select 42" {
		t.Fatalf("first SQL tab was not preserved: active=%s buffer=%q", model.workspace.ActiveTab(), model.sqlTabs[firstTab].Buffer())
	}
}

func TestReconnectReplacesSessionAndPreservesWorkspaceTabs(t *testing.T) {
	service, connector := connectionFixture()
	oldSession := connector.session
	model := loadLauncher(t, New(Dependencies{Profiles: service, Connector: connector}))
	model, command := beginQuickConnect(t, model)
	model, command = updateApp(t, model, command())

	// Open and edit SQL before the connection is lost.
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model, command = updateApp(t, model, tea.KeyPressMsg{Code: 'n'})
	model, _ = updateApp(t, model, command())
	tabID := model.workspace.ActiveTab()
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 's', Text: "select 1"})

	oldSession.pingError = core.NewError(
		"ping", core.ErrorNetwork, "PostgreSQL closed the connection.", true, nil,
	)
	model, command = updateApp(t, model, model.workspace.CheckConnectionNow())
	model, command = updateApp(t, model, command())
	model, _ = updateApp(t, model, command())
	if model.workspace.Connection() != workspace.ConnectionLost {
		t.Fatalf("connection state = %s", model.workspace.Connection())
	}

	newSession := &fakeSession{}
	connector.session = newSession
	model, command = updateApp(t, model, tea.KeyPressMsg{Code: 'r'})
	model, command = updateApp(t, model, command())
	model, _ = updateApp(t, model, command())
	if model.session != newSession || oldSession.closes != 1 {
		t.Fatalf("session replacement: current=%p new=%p old closes=%d", model.session, newSession, oldSession.closes)
	}
	if model.workspace.Connection() != workspace.ConnectionConnected || len(model.workspace.Tabs()) != 1 || model.workspace.ActiveTab() != tabID {
		t.Fatalf("workspace after reconnect: state=%s tabs=%#v active=%s", model.workspace.Connection(), model.workspace.Tabs(), model.workspace.ActiveTab())
	}
	if model.executor != newSession || model.sqlTabs[tabID].Buffer() != "select 1" {
		t.Fatalf("SQL reconnect state: executor=%T buffer=%q", model.executor, model.sqlTabs[tabID].Buffer())
	}
}
