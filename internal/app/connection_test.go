package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/launcher"
	"github.com/Jason-Wang1245/db-tui/internal/profile"
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

type fakeSession struct{ closes int }

func (*fakeSession) Ping(context.Context) error { return nil }
func (session *fakeSession) Close()             { session.closes++ }

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
