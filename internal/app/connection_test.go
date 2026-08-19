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
	closes    int
	pingError error
	schemas   []workspace.Schema
	relations map[string][]workspace.Relation
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
		CanSelect: true, HasXMin: true, ReadOnlyReason: "Browsing only.",
		Columns: []grid.Column{
			{Name: "id", DataType: "bigint", TypeOID: 20, CanSelect: true, Sortable: true, IdentityPart: true},
			{Name: "email", DataType: "text", TypeOID: 25, CanSelect: true, Sortable: true},
		},
	}, nil
}

func (*fakeSession) FetchPage(_ context.Context, _ grid.Relation, request grid.PageRequest) (grid.Page, error) {
	return grid.Page{Rows: []grid.Row{{
		Identity: map[string]any{"id": int64(1)}, XMin: 7,
		Cells: []grid.Cell{{Raw: int64(1), Display: "1"}, {Raw: "a@example.com", Display: "a@example.com"}},
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

	model, command = updateApp(t, model, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	model, command = updateApp(t, model, command())
	if model.surface != surfaceLauncher || model.workspaceReady || model.session != nil {
		t.Fatalf("disconnect state: surface=%s ready=%v session=%v", model.surface, model.workspaceReady, model.session)
	}
}

func TestReconnectReplacesSessionAndPreservesWorkspaceTabs(t *testing.T) {
	service, connector := connectionFixture()
	oldSession := connector.session
	model := loadLauncher(t, New(Dependencies{Profiles: service, Connector: connector}))
	model, command := beginQuickConnect(t, model)
	model, command = updateApp(t, model, command())

	// Open an SQL placeholder before the connection is lost.
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model, _ = updateApp(t, model, tea.KeyPressMsg{Code: 'n'})
	tabID := model.workspace.ActiveTab()

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
}
