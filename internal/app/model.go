// Package app owns the root Bubble Tea model and application lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/launcher"
	"github.com/Jason-Wang1245/db-tui/internal/profile"
	"github.com/Jason-Wang1245/db-tui/internal/ui"
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
	width         int
	height        int
	diagnostics   *Diagnostics
	theme         ui.Theme
	launcher      launcher.Model
	launcherReady bool
	profiles      ProfileService
	connector     launcher.Connector
	cancellations *CancellationRegistry
	surface       surface
	session       launcher.Session
	connected     launcher.Profile
	connection    launcher.ConnectionInfo
	warning       *core.Error
}

type connectionSucceededMsg struct {
	profile launcher.Profile
	session launcher.Session
	info    launcher.ConnectionInfo
	warning *core.Error
	request core.RequestID
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
	case connectionSucceededMsg:
		if !model.launcher.Expects(launcher.ActionConnect, message.request) {
			message.session.Close()
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
		model.diagnostics.Record(Diagnostic{Operation: "connect", Status: "success"})
		return model, nil
	case tea.KeyPressMsg:
		if message.Keystroke() == "ctrl+c" {
			return model, model.shutdownCommand()
		}
		if model.surface == surfaceWorkspace {
			switch message.Keystroke() {
			case "q":
				return model, model.shutdownCommand()
			case "esc":
				session := model.session
				model.session = nil
				model.surface = surfaceLauncher
				model.warning = nil
				return model, tea.Sequence(closeSessionCommand(session), model.launcher.Reload())
			}
			return model, nil
		}
		if model.launcherReady && message.Keystroke() == "q" && model.launcher.CanQuit() {
			return model, model.shutdownCommand()
		}
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
		content = strings.Join([]string{
			model.theme.Title.Render("db-tui"), "",
			"Terminal is too small for safe editing.",
			fmt.Sprintf("Need at least 48×12; current size is %d×%d.", model.width, model.height),
			"Resize to continue. Current state is preserved.", "",
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
	lines := []string{
		model.theme.Title.Render("db-tui"), "",
		model.theme.Title.Render(model.connected.Name),
		fmt.Sprintf("%s / %s • PostgreSQL %s", model.connection.Server, model.connection.Database, model.connection.ServerVersion),
		"",
		"Connected workspace shell is ready for the catalog implementation slice.", "",
		model.theme.Muted.Render("Esc disconnect • q quit"),
	}
	if model.warning != nil {
		lines = append(lines, "", model.warning.Summary)
	}
	return strings.Join(lines, "\n")
}
