package launcher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: string(code)}
}

func ctrl(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: string(code), Mod: tea.ModCtrl}
}

func loadedModel(t *testing.T, profiles ...Profile) Model {
	t.Helper()
	model := NewModel()
	initMessage := model.Init()()
	if _, ok := initMessage.(LoadProfilesIntent); !ok {
		t.Fatalf("Init message = %T", initMessage)
	}
	model.Update(ProfilesLoadedMsg{Profiles: profiles, Request: 1})
	return model
}

func TestURLImportIntentAndFormNeverRenderPassword(t *testing.T) {
	model := loadedModel(t)
	model.Update(key('n'))
	secret := "never-show-this"
	raw := "postgresql://alice:" + secret + "@localhost/app"
	model.inputs[fieldURL].SetValue(raw)
	if strings.Contains(model.View(), secret) {
		t.Fatal("URL password was rendered")
	}
	command := model.Update(ctrl('u'))
	intent, ok := command().(ImportURLIntent)
	if !ok || intent.Raw != raw {
		t.Fatalf("import intent = %#v", intent)
	}
	draft := NewDraft()
	draft.Name = "app@localhost"
	draft.Host = "localhost"
	draft.Database = "app"
	draft.User = "alice"
	draft.Password = secret
	draft.ReplacePassword = true
	model.Update(URLImportedMsg{Draft: draft, Request: intent.Request})
	if model.inputs[fieldURL].Value() != "" || strings.Contains(model.View(), secret) {
		t.Fatal("imported URL or password remained visible")
	}
}

func TestEmptyPasswordRequiresExplicitEnter(t *testing.T) {
	model := loadedModel(t)
	model.Update(key('n'))
	model.focus(fieldPassword)
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	command := model.Update(ctrl('t'))
	intent, ok := command().(TestConnectionIntent)
	if !ok {
		t.Fatalf("test command = %T", command())
	}
	if !intent.Draft.ReplacePassword || intent.Draft.Password != "" {
		t.Fatalf("empty password was not explicit: %#v", intent.Draft)
	}
}

func TestCancelledTestCanBeRunAgainAndDraftIsPreserved(t *testing.T) {
	model := loadedModel(t)
	model.Update(key('n'))
	model.inputs[fieldName].SetValue("Local")
	model.inputs[fieldHost].SetValue("localhost")
	model.inputs[fieldDatabase].SetValue("app")
	model.inputs[fieldUser].SetValue("developer")
	model.inputs[fieldPassword].SetValue("secret")
	model.draft.ReplacePassword = true

	first := model.Update(ctrl('t'))().(TestConnectionIntent)
	cancel := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})().(CancelIntent)
	if cancel.Request != first.Request || cancel.Action != ActionTest {
		t.Fatalf("cancel intent = %#v", cancel)
	}
	model.Update(OperationFailedMsg{
		Action: ActionTest, Request: first.Request,
		Err: core.NewError("test", core.ErrorCancellation, "Operation cancelled. You can run it again.", true, context.Canceled),
	})
	second := model.Update(ctrl('t'))().(TestConnectionIntent)
	if second.Request == first.Request || second.Draft.Password != "secret" {
		t.Fatalf("rerun = %#v", second)
	}
}

func TestStaleResultsDoNotOverwriteCurrentOperation(t *testing.T) {
	model := NewModel()
	model.Update(ProfilesLoadedMsg{Profiles: []Profile{{ID: "stale"}}, Request: 99})
	if model.CanQuit() {
		t.Fatal("stale load result cleared the active request")
	}
	model.Update(ProfilesLoadedMsg{Profiles: []Profile{{ID: "current", Name: "Current"}}, Request: 1})
	if !model.CanQuit() || model.selected().ID != "current" {
		t.Fatalf("current load was not applied: %#v", model)
	}
}

func TestDeleteRequiresNamedConfirmation(t *testing.T) {
	model := loadedModel(t, Profile{ID: "one", Name: "Production"})
	if command := model.Update(key('d')); command != nil {
		t.Fatal("delete key should only open confirmation")
	}
	if !strings.Contains(model.View(), "Delete Production?") {
		t.Fatalf("confirmation view = %q", model.View())
	}
	intent, ok := model.Update(key('y'))().(DeleteProfileIntent)
	if !ok || intent.ID != "one" {
		t.Fatalf("delete intent = %#v", intent)
	}
}

func TestDeleteConfirmationDefaultsToCancel(t *testing.T) {
	model := loadedModel(t, Profile{ID: "one", Name: "Production"})
	model.Update(key('d'))
	if command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); command != nil {
		t.Fatal("Enter unexpectedly confirmed destructive deletion")
	}
	if model.deleteConfirm || !strings.Contains(model.View(), "Deletion cancelled") {
		t.Fatalf("safe-default state = %#v, view = %q", model.state, model.View())
	}
}

func TestMouseButtonsUseTheSameIntentsAsKeyboard(t *testing.T) {
	model := loadedModel(t, Profile{ID: "one", Name: "Production"})
	model.SetHitboxes([]Hitbox{
		{Rect: ui.Rect{X: 0, Y: 0, Width: 5, Height: 1}, Action: MouseSelect, Profile: 0},
		{Rect: ui.Rect{X: 10, Y: 0, Width: 9, Height: 1}, Action: MouseConnect},
	})
	model.Update(tea.MouseClickMsg{X: 1, Y: 0, Button: tea.MouseLeft})
	command := model.Update(tea.MouseClickMsg{X: 12, Y: 0, Button: tea.MouseLeft})
	intent, ok := command().(LoadDraftIntent)
	if !ok || intent.ID != "one" || intent.Then != ActionConnect {
		t.Fatalf("mouse connect intent = %#v", intent)
	}
}

func TestLongProfileListKeepsSelectionVisible(t *testing.T) {
	profiles := make([]Profile, 20)
	for index := range profiles {
		profiles[index] = Profile{ID: fmt.Sprintf("profile-%d", index), Name: fmt.Sprintf("Profile %d", index)}
	}
	model := loadedModel(t, profiles...)
	model.SetSize(80, 12)
	for range 6 {
		model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if model.profileOffset == 0 || !strings.Contains(model.View(), "Profile 6") {
		t.Fatalf("selection=%d offset=%d view=%q", model.state.SelectedIndex, model.profileOffset, model.View())
	}
}

func TestErrorsShowPostgreSQLRecoveryDetails(t *testing.T) {
	model := loadedModel(t)
	model.Update(key('n'))
	intent := model.Update(ctrl('t'))().(TestConnectionIntent)
	classified := core.NewError("connect", core.ErrorValidation, "PostgreSQL rejected the value.", false, errors.New("driver"))
	classified.PostgreSQL = &core.PostgreSQLDetails{SQLState: "22P02", Detail: "invalid input syntax", Hint: "Use a valid value"}
	model.Update(OperationFailedMsg{Action: ActionTest, Request: intent.Request, Err: classified})
	view := model.View()
	for _, expected := range []string{"PostgreSQL rejected", "invalid input syntax", "Use a valid value", "22P02"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view %q does not contain %q", view, expected)
		}
	}
}
