package launcher

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

type Action string

const (
	ActionLoad    Action = "load"
	ActionEdit    Action = "edit"
	ActionSave    Action = "save"
	ActionDelete  Action = "delete"
	ActionTest    Action = "test"
	ActionConnect Action = "connect"
)

type Profile struct {
	ID           string
	Name         string
	Host         string
	Port         uint16
	Database     string
	User         string
	SSLMode      string
	Advanced     map[string]string
	SavePassword bool
	LastUsedAt   time.Time
}

type Parameter struct {
	Name  string
	Value string
}

type Draft struct {
	ID                 string
	Name               string
	Host               string
	Port               string
	Database           string
	User               string
	Password           string
	ReplacePassword    bool
	HasStoredPassword  bool
	SavePassword       bool
	SSLMode            string
	AdvancedParameters []Parameter
}

func NewDraft() Draft {
	return Draft{Port: "5432", SSLMode: "prefer", SavePassword: true}
}

type LoadProfilesIntent struct{ Request core.RequestID }

type LoadDraftIntent struct {
	ID      string
	Then    Action
	Request core.RequestID
}

type SaveProfileIntent struct {
	Draft   Draft
	Request core.RequestID
}

type DeleteProfileIntent struct {
	ID      string
	Request core.RequestID
}

type TestConnectionIntent struct {
	Draft   Draft
	Request core.RequestID
}

type ConnectIntent struct {
	Draft   Draft
	Request core.RequestID
}

type ImportURLIntent struct {
	Raw     string
	Request core.RequestID
}

type CancelIntent struct {
	Action  Action
	Request core.RequestID
}

type ProfilesLoadedMsg struct {
	Profiles []Profile
	Request  core.RequestID
}

type DraftLoadedMsg struct {
	Draft   Draft
	Then    Action
	Request core.RequestID
}

type ProfileSavedMsg struct {
	Profile Profile
	Warning *core.Error
	Request core.RequestID
}

type ProfileDeletedMsg struct{ Request core.RequestID }

type ConnectionTestedMsg struct {
	Info    ConnectionInfo
	Warning *core.Error
	Request core.RequestID
}

type URLImportedMsg struct {
	Draft   Draft
	Request core.RequestID
}

type PasswordRequiredMsg struct {
	Draft   Draft
	Then    Action
	Request core.RequestID
}

type OperationFailedMsg struct {
	Action  Action
	Err     error
	Request core.RequestID
}

type ConnectedMsg struct{ Request core.RequestID }

const (
	fieldURL = iota
	fieldName
	fieldHost
	fieldPort
	fieldDatabase
	fieldUser
	fieldPassword
	fieldSSLMode
	fieldAdvanced
	fieldCount
)

type activeOperation struct {
	action  Action
	origin  Mode
	request core.RequestID
}

type Model struct {
	theme         ui.Theme
	width         int
	height        int
	state         State
	profiles      []Profile
	profileOffset int
	draft         Draft
	inputs        [fieldCount]textinput.Model
	focused       int
	deleteConfirm bool
	status        string
	err           error
	active        activeOperation
	nextRequest   core.RequestID
	hitboxes      []Hitbox
}

type MouseAction string

const (
	MouseSelect  MouseAction = "select"
	MouseNew     MouseAction = "new"
	MouseEdit    MouseAction = "edit"
	MouseTest    MouseAction = "test"
	MouseConnect MouseAction = "connect"
	MouseDelete  MouseAction = "delete"
)

type Hitbox struct {
	Rect    ui.Rect
	Action  MouseAction
	Profile int
}

func NewModel() Model {
	model := Model{theme: ui.DefaultTheme(), state: NewState(), nextRequest: 1}
	model.active = activeOperation{action: ActionLoad, origin: ModeProfiles, request: 1}
	model.initializeInputs()
	return model
}

func (model *Model) initializeInputs() {
	placeholders := [fieldCount]string{
		"postgresql://user:password@host:5432/database?sslmode=prefer",
		"Local development", "localhost", "5432", "postgres", "postgres",
		"password", "prefer", "application_name=my-tool&search_path=public",
	}
	for index := range model.inputs {
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = placeholders[index]
		input.SetWidth(70)
		model.inputs[index] = input
	}
	model.inputs[fieldURL].EchoMode = textinput.EchoPassword
	model.inputs[fieldPassword].EchoMode = textinput.EchoPassword
}

func (model Model) Init() tea.Cmd {
	intent := LoadProfilesIntent{Request: 1}
	return func() tea.Msg { return intent }
}

func (model *Model) SetSize(width, height int) {
	model.width = width
	model.height = height
	inputWidth := width - 32
	if inputWidth > 90 {
		inputWidth = 90
	}
	if inputWidth < 12 {
		inputWidth = 12
	}
	for index := range model.inputs {
		model.inputs[index].SetWidth(inputWidth)
	}
	model.ensureSelectionVisible()
}

func (model *Model) SetHitboxes(hitboxes []Hitbox) {
	model.hitboxes = append(model.hitboxes[:0], hitboxes...)
}

func (model *Model) CanQuit() bool {
	return model.state.Mode == ModeProfiles && model.active.action == "" && !model.deleteConfirm
}

func (model *Model) Expects(action Action, request core.RequestID) bool {
	return model.matches(action, request)
}

func (model *Model) Reload() tea.Cmd {
	request := model.newRequest()
	model.active = activeOperation{action: ActionLoad, origin: ModeProfiles, request: request}
	intent := LoadProfilesIntent{Request: request}
	return func() tea.Msg { return intent }
}

func (model *Model) Update(message tea.Msg) tea.Cmd {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.SetSize(message.Width, message.Height)
		return nil
	case ProfilesLoadedMsg:
		if !model.matches(ActionLoad, message.Request) {
			return nil
		}
		model.clearOperation()
		model.profiles = message.Profiles
		model.clampSelection()
		return nil
	case DraftLoadedMsg:
		if !model.matches(ActionEdit, message.Request) {
			return nil
		}
		model.clearOperation()
		if message.Then == ActionEdit {
			model.openForm(message.Draft)
			return nil
		}
		return model.startConnectionIntent(message.Then, message.Draft)
	case ProfileSavedMsg:
		if !model.matches(ActionSave, message.Request) {
			return nil
		}
		model.clearOperation()
		model.clearSensitiveInputs()
		model.state.Mode = ModeProfiles
		model.status = "Saved " + message.Profile.Name + "."
		if message.Warning != nil {
			model.status = message.Warning.Summary
		}
		return model.Reload()
	case ProfileDeletedMsg:
		if !model.matches(ActionDelete, message.Request) {
			return nil
		}
		model.clearOperation()
		model.deleteConfirm = false
		model.status = "Connection profile deleted."
		return model.Reload()
	case ConnectionTestedMsg:
		if !model.matches(ActionTest, message.Request) {
			return nil
		}
		model.clearOperation()
		model.status = fmt.Sprintf(
			"Connected to %s / %s (PostgreSQL %s, %s).",
			message.Info.Server, message.Info.Database, message.Info.ServerVersion, formatLatency(message.Info.Latency),
		)
		if message.Warning != nil {
			model.status += " " + message.Warning.Summary
		}
		return nil
	case URLImportedMsg:
		if !model.matches(ActionEdit, message.Request) {
			return nil
		}
		model.clearOperation()
		model.draft = message.Draft
		model.setInputs(message.Draft)
		model.inputs[fieldURL].SetValue("")
		model.focus(fieldName)
		model.status = "URL imported into fields. The original URL will not be saved."
		return nil
	case PasswordRequiredMsg:
		if !model.matches(message.Then, message.Request) {
			return nil
		}
		model.clearOperation()
		model.openForm(message.Draft)
		model.focus(fieldPassword)
		model.status = "Enter a password. To use an empty password explicitly, press Enter in the password field."
		return nil
	case OperationFailedMsg:
		if !model.matches(message.Action, message.Request) {
			return nil
		}
		model.clearOperation()
		model.status = ""
		model.err = message.Err
		return nil
	case ConnectedMsg:
		if model.matches(ActionConnect, message.Request) {
			model.clearOperation()
			model.clearSensitiveInputs()
		}
		return nil
	case tea.MouseClickMsg:
		return model.updateMouse(message)
	case tea.MouseWheelMsg:
		if model.state.Mode == ModeProfiles && model.active.action == "" {
			if message.Mouse().Button == tea.MouseWheelUp {
				model.moveSelection(-1)
			} else if message.Mouse().Button == tea.MouseWheelDown {
				model.moveSelection(1)
			}
		}
		return nil
	case tea.KeyPressMsg:
		return model.updateKey(message)
	default:
		if model.state.Mode == ModeEdit && model.active.action == "" {
			return model.updateFocusedInput(message)
		}
	}
	return nil
}

func (model *Model) updateKey(message tea.KeyPressMsg) tea.Cmd {
	key := message.Keystroke()
	if model.active.action != "" {
		if key == "esc" {
			active := model.active
			model.status = "Cancelling " + string(active.action) + "…"
			intent := CancelIntent{Action: active.action, Request: active.request}
			return func() tea.Msg { return intent }
		}
		return nil
	}
	model.err = nil
	if model.state.Mode == ModeProfiles {
		return model.updateProfilesKey(key)
	}
	return model.updateFormKey(message)
}

func (model *Model) updateProfilesKey(key string) tea.Cmd {
	if model.deleteConfirm {
		switch key {
		case "y":
			return model.startDelete()
		case "n", "esc", "enter":
			model.deleteConfirm = false
			model.status = "Deletion cancelled."
		}
		return nil
	}
	switch key {
	case "up", "k":
		model.moveSelection(-1)
	case "down", "j":
		model.moveSelection(1)
	case "n":
		model.openForm(NewDraft())
	case "e":
		return model.loadSelectedDraft(ActionEdit)
	case "t":
		return model.loadSelectedDraft(ActionTest)
	case "enter", "c":
		return model.loadSelectedDraft(ActionConnect)
	case "d":
		if selected := model.selected(); selected != nil {
			model.deleteConfirm = true
			model.status = "Delete " + selected.Name + "? Press y to confirm or n to cancel."
		}
	case "r":
		return model.Reload()
	}
	return nil
}

func (model *Model) updateFormKey(message tea.KeyPressMsg) tea.Cmd {
	switch message.Keystroke() {
	case "esc":
		model.closeForm()
		return nil
	case "tab", "down":
		model.focus(model.nextField(1))
		return nil
	case "shift+tab", "up":
		model.focus(model.nextField(-1))
		return nil
	case "ctrl+p":
		model.draft.SavePassword = !model.draft.SavePassword
		return nil
	case "ctrl+u":
		if model.draft.ID != "" {
			model.err = errors.New("URL import is available only while creating a profile")
			return nil
		}
		request := model.newRequest()
		model.active = activeOperation{action: ActionEdit, origin: ModeEdit, request: request}
		raw := model.inputs[fieldURL].Value()
		intent := ImportURLIntent{Raw: raw, Request: request}
		return func() tea.Msg { return intent }
	case "ctrl+s":
		draft, err := model.currentDraft()
		if err != nil {
			model.err = err
			return nil
		}
		return model.startSave(draft)
	case "ctrl+t":
		draft, err := model.currentDraft()
		if err != nil {
			model.err = err
			return nil
		}
		return model.startConnectionIntent(ActionTest, draft)
	case "ctrl+enter":
		draft, err := model.currentDraft()
		if err != nil {
			model.err = err
			return nil
		}
		return model.startConnectionIntent(ActionConnect, draft)
	case "enter":
		if model.focused == fieldPassword {
			model.draft.ReplacePassword = true
			if model.inputs[fieldPassword].Value() == "" {
				model.status = "Empty password confirmed. Use Ctrl+T to test or Ctrl+Enter to connect."
			} else {
				model.status = "Password replacement ready. Use Ctrl+T to test or Ctrl+Enter to connect."
			}
			return nil
		}
	}
	return model.updateFocusedInput(message)
}

func (model *Model) updateFocusedInput(message tea.Msg) tea.Cmd {
	before := model.inputs[model.focused].Value()
	updated, command := model.inputs[model.focused].Update(message)
	model.inputs[model.focused] = updated
	if model.focused == fieldPassword && updated.Value() != before {
		model.draft.ReplacePassword = true
	}
	return command
}

func (model *Model) openForm(draft Draft) {
	model.state.Mode = ModeEdit
	model.draft = draft
	model.deleteConfirm = false
	model.err = nil
	model.status = ""
	model.setInputs(draft)
	model.focus(fieldName)
}

func (model *Model) closeForm() {
	for index := range model.inputs {
		model.inputs[index].SetValue("")
		model.inputs[index].Blur()
	}
	model.draft.Password = ""
	model.state.Mode = ModeProfiles
	model.status = "Edits discarded."
	model.err = nil
}

func (model *Model) clearSensitiveInputs() {
	model.inputs[fieldURL].SetValue("")
	model.inputs[fieldPassword].SetValue("")
	model.draft.Password = ""
}

func (model *Model) setInputs(draft Draft) {
	model.inputs[fieldName].SetValue(draft.Name)
	model.inputs[fieldHost].SetValue(draft.Host)
	model.inputs[fieldPort].SetValue(draft.Port)
	model.inputs[fieldDatabase].SetValue(draft.Database)
	model.inputs[fieldUser].SetValue(draft.User)
	model.inputs[fieldPassword].SetValue(draft.Password)
	model.inputs[fieldSSLMode].SetValue(draft.SSLMode)
	model.inputs[fieldAdvanced].SetValue(formatAdvanced(draft.AdvancedParameters))
}

func (model *Model) currentDraft() (Draft, error) {
	parameters, err := parseAdvanced(model.inputs[fieldAdvanced].Value())
	if err != nil {
		return Draft{}, err
	}
	draft := model.draft
	draft.Name = model.inputs[fieldName].Value()
	draft.Host = model.inputs[fieldHost].Value()
	draft.Port = model.inputs[fieldPort].Value()
	draft.Database = model.inputs[fieldDatabase].Value()
	draft.User = model.inputs[fieldUser].Value()
	draft.Password = model.inputs[fieldPassword].Value()
	draft.SSLMode = model.inputs[fieldSSLMode].Value()
	draft.AdvancedParameters = parameters
	return draft, nil
}

func parseAdvanced(raw string) ([]Parameter, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, "&")
	parameters := make([]Parameter, 0, len(parts))
	for index, part := range parts {
		name, value, found := strings.Cut(part, "=")
		if !found {
			return nil, fmt.Errorf("advanced[%d]: must use name=value", index)
		}
		decodedName, nameErr := url.QueryUnescape(name)
		decodedValue, valueErr := url.QueryUnescape(value)
		if nameErr != nil || valueErr != nil {
			return nil, fmt.Errorf("advanced[%d]: contains invalid URL escaping", index)
		}
		parameters = append(parameters, Parameter{Name: decodedName, Value: decodedValue})
	}
	return parameters, nil
}

func formatAdvanced(parameters []Parameter) string {
	parts := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		parts = append(parts, url.QueryEscape(parameter.Name)+"="+url.QueryEscape(parameter.Value))
	}
	return strings.Join(parts, "&")
}

func (model *Model) focus(field int) {
	for index := range model.inputs {
		model.inputs[index].Blur()
	}
	model.focused = field
	model.inputs[field].Focus()
}

func (model *Model) nextField(direction int) int {
	next := model.focused
	for {
		next = (next + direction + fieldCount) % fieldCount
		if next != fieldURL || model.draft.ID == "" {
			return next
		}
	}
}

func (model *Model) loadSelectedDraft(next Action) tea.Cmd {
	selected := model.selected()
	if selected == nil {
		return nil
	}
	request := model.newRequest()
	model.active = activeOperation{action: ActionEdit, origin: ModeProfiles, request: request}
	model.status = "Loading profile…"
	intent := LoadDraftIntent{ID: selected.ID, Then: next, Request: request}
	return func() tea.Msg { return intent }
}

func (model *Model) startSave(draft Draft) tea.Cmd {
	request := model.newRequest()
	model.active = activeOperation{action: ActionSave, origin: model.state.Mode, request: request}
	model.status = "Saving profile…"
	intent := SaveProfileIntent{Draft: draft, Request: request}
	return func() tea.Msg { return intent }
}

func (model *Model) startDelete() tea.Cmd {
	selected := model.selected()
	if selected == nil {
		return nil
	}
	request := model.newRequest()
	model.active = activeOperation{action: ActionDelete, origin: ModeProfiles, request: request}
	model.status = "Deleting profile and saved password…"
	intent := DeleteProfileIntent{ID: selected.ID, Request: request}
	return func() tea.Msg { return intent }
}

func (model *Model) startConnectionIntent(next Action, draft Draft) tea.Cmd {
	request := model.newRequest()
	model.active = activeOperation{action: next, origin: model.state.Mode, request: request}
	if next == ActionTest {
		model.status = "Testing connection…"
		intent := TestConnectionIntent{Draft: draft, Request: request}
		return func() tea.Msg { return intent }
	}
	model.status = "Connecting…"
	intent := ConnectIntent{Draft: draft, Request: request}
	return func() tea.Msg { return intent }
}

func (model *Model) newRequest() core.RequestID {
	model.nextRequest++
	return model.nextRequest
}

func (model *Model) matches(next Action, request core.RequestID) bool {
	return model.active.action == next && model.active.request == request
}

func (model *Model) clearOperation() { model.active = activeOperation{} }

func (model *Model) selected() *Profile {
	if len(model.profiles) == 0 {
		return nil
	}
	return &model.profiles[model.state.SelectedIndex]
}

func (model *Model) moveSelection(delta int) {
	if len(model.profiles) != 0 {
		model.state.SelectedIndex = (model.state.SelectedIndex + delta + len(model.profiles)) % len(model.profiles)
		model.ensureSelectionVisible()
	}
}

func (model *Model) clampSelection() {
	if len(model.profiles) == 0 {
		model.state.SelectedIndex = 0
	} else if model.state.SelectedIndex >= len(model.profiles) {
		model.state.SelectedIndex = len(model.profiles) - 1
	}
	model.ensureSelectionVisible()
}

func (model *Model) ensureSelectionVisible() {
	visibleRows := model.visibleProfileRows()
	if model.state.SelectedIndex < model.profileOffset {
		model.profileOffset = model.state.SelectedIndex
	}
	if model.state.SelectedIndex >= model.profileOffset+visibleRows {
		model.profileOffset = model.state.SelectedIndex - visibleRows + 1
	}
	maximumOffset := max(0, len(model.profiles)-visibleRows)
	if model.profileOffset > maximumOffset {
		model.profileOffset = maximumOffset
	}
}

func (model Model) visibleProfileRows() int {
	if model.height <= 0 {
		return max(1, len(model.profiles))
	}
	return max(1, model.height-9)
}

func (model *Model) updateMouse(message tea.MouseClickMsg) tea.Cmd {
	if message.Button != tea.MouseLeft || model.state.Mode != ModeProfiles || model.active.action != "" {
		return nil
	}
	for _, hitbox := range model.hitboxes {
		if !hitbox.Rect.Contains(message.X, message.Y) {
			continue
		}
		switch hitbox.Action {
		case MouseSelect:
			index := model.profileOffset + hitbox.Profile
			if index >= 0 && index < len(model.profiles) {
				model.state.SelectedIndex = index
			}
		case MouseNew:
			model.openForm(NewDraft())
		case MouseEdit:
			return model.loadSelectedDraft(ActionEdit)
		case MouseTest:
			return model.loadSelectedDraft(ActionTest)
		case MouseConnect:
			return model.loadSelectedDraft(ActionConnect)
		case MouseDelete:
			if selected := model.selected(); selected != nil {
				model.deleteConfirm = true
				model.status = "Delete " + selected.Name + "? Press y to confirm or n to cancel."
			}
		}
		return nil
	}
	return nil
}

func (model Model) View() string {
	if model.state.Mode == ModeEdit {
		return model.formView()
	}
	return model.profilesView()
}

func (model Model) profilesView() string {
	hints := "n new • e edit • t test • enter connect • d delete • r refresh • q quit"
	if model.width > 0 && model.width < 80 {
		hints = "n new • e edit • t test • d delete • ↵ connect"
	}
	lines := []string{
		model.theme.Title.Render("db-tui"), "", model.theme.Title.Render("Connections"),
		"[New]  [Edit]  [Test]  [Connect]  [Delete]",
		model.theme.Muted.Render(hints), "",
	}
	compact := model.width > 0 && model.width < 80
	if compact {
		lines = append(lines, "  Name — server / database [SSL]")
	} else {
		lines = append(lines, fmt.Sprintf("  %-24s %-34s %-12s", "Name", "Server / database", "SSL"))
	}
	if len(model.profiles) == 0 && model.active.action == "" {
		lines = append(lines, "", "No saved connections. Press n to create one.")
	} else {
		limit := min(len(model.profiles), model.profileOffset+model.visibleProfileRows())
		for index := model.profileOffset; index < limit; index++ {
			saved := model.profiles[index]
			marker := " "
			if index == model.state.SelectedIndex {
				marker = ">"
			}
			server := fmt.Sprintf("%s:%d/%s", saved.Host, saved.Port, saved.Database)
			if compact {
				width := model.width
				if width < 1 {
					width = 48
				}
				lines = append(lines, truncate(fmt.Sprintf("%s %s — %s [%s]", marker, saved.Name, server, saved.SSLMode), width))
			} else {
				lines = append(lines, fmt.Sprintf("%s %-24s %-34s %-12s", marker, truncate(saved.Name, 24), truncate(server, 34), saved.SSLMode))
			}
		}
	}
	return model.withFeedback(lines)
}

func (model Model) formView() string {
	title := "New connection"
	if model.draft.ID != "" {
		title = "Edit connection"
	}
	lines := []string{model.theme.Title.Render("db-tui"), "", model.theme.Title.Render(title)}
	if model.draft.ID == "" {
		urlLabel := "  URL import (masked; Ctrl+U)"
		if model.focused == fieldURL {
			urlLabel = "› URL import (masked; Ctrl+U)"
		}
		lines = append(lines, fmt.Sprintf("%-30s %s", urlLabel, model.inputs[fieldURL].View()))
	}
	labels := []struct {
		field int
		label string
	}{
		{fieldName, "Name"}, {fieldHost, "Host"}, {fieldPort, "Port"}, {fieldDatabase, "Database"},
		{fieldUser, "User"}, {fieldPassword, "Password"}, {fieldSSLMode, "SSL mode"},
		{fieldAdvanced, "Advanced parameters (URL-query format)"},
	}
	for _, item := range labels {
		label := item.label
		if item.field == fieldPassword && model.draft.HasStoredPassword && !model.draft.ReplacePassword {
			label += " [stored]"
		}
		if item.field == model.focused {
			label = "› " + label
		} else {
			label = "  " + label
		}
		lines = append(lines, fmt.Sprintf("%-30s %s", truncate(label, 30), model.inputs[item.field].View()))
	}
	passwordMode := "off (session only)"
	if model.draft.SavePassword {
		passwordMode = "on (system keychain)"
	}
	lines = append(lines,
		"Save password: "+passwordMode+" (Ctrl+P toggles)",
		model.theme.Muted.Render("Ctrl+S save • Ctrl+T test • Ctrl+Enter connect • Tab move • Esc discard"),
	)
	return model.withFeedback(lines)
}

func (model Model) withFeedback(lines []string) string {
	if model.active.action != "" && model.status == "" {
		lines = append(lines, "Working… Esc cancel")
	} else if model.status != "" {
		lines = append(lines, model.status)
	}
	if model.err != nil {
		lines = append(lines, renderError(model.err))
	}
	if model.deleteConfirm {
		lines = append(lines, "", "Deletion also removes the keychain entry. y confirm • n cancel")
	}
	return strings.Join(lines, "\n")
}

func renderError(err error) string {
	var classified *core.Error
	if errors.As(err, &classified) {
		lines := []string{classified.Summary}
		if classified.PostgreSQL != nil {
			if classified.PostgreSQL.Detail != "" {
				lines = append(lines, "Detail: "+classified.PostgreSQL.Detail)
			}
			if classified.PostgreSQL.Hint != "" {
				lines = append(lines, "Hint: "+classified.PostgreSQL.Hint)
			}
			if classified.PostgreSQL.SQLState != "" {
				lines = append(lines, "SQLSTATE: "+classified.PostgreSQL.SQLState)
			}
		}
		return strings.Join(lines, "\n")
	}
	return err.Error()
}

func truncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func formatLatency(latency time.Duration) string {
	if latency < time.Millisecond {
		return "<1ms"
	}
	return latency.Round(time.Millisecond).String()
}
