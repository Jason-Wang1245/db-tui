package profile

import (
	"errors"
	"testing"
)

func validDraft() Draft {
	return Draft{
		Name: "Local", Host: "localhost", Port: "5432", Database: "app",
		User: "developer", SSLMode: "prefer", SavePassword: true,
	}
}

func TestValidateDraftCanonicalizesAndRejectsDuplicates(t *testing.T) {
	draft := validDraft()
	draft.Name = " Local "
	draft.SSLMode = "REQUIRE"
	draft.AdvancedParameters = []Parameter{{Name: " Search_Path ", Value: "private,public"}}
	document := Document{Profiles: []Profile{{ID: "other", Name: "LOCAL"}}}

	_, err := ValidateDraft(draft, document)
	var validation ValidationErrors
	if !errors.As(err, &validation) || validation.For("name") != "must be unique" {
		t.Fatalf("ValidateDraft error = %#v, want duplicate-name validation", err)
	}

	draft.ID = "other"
	saved, err := ValidateDraft(draft, document)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Local" || saved.SSLMode != "require" || saved.AdvancedParameters["search_path"] != "private,public" {
		t.Fatalf("canonical profile = %#v", saved)
	}
}

func TestValidateDraftRejectsCoreAndSecretParameters(t *testing.T) {
	for _, name := range []string{"password", "passfile", "sslpassword", "host", "port", "user", "database", "sslmode", "connect_timeout", "api_secret"} {
		t.Run(name, func(t *testing.T) {
			draft := validDraft()
			draft.AdvancedParameters = []Parameter{{Name: name, Value: "not-persisted"}}
			if _, err := ValidateDraft(draft, Document{}); err == nil {
				t.Fatalf("parameter %q unexpectedly accepted", name)
			}
		})
	}
}

func TestValidateDraftRejectsSecretInAdvancedValue(t *testing.T) {
	draft := validDraft()
	draft.AdvancedParameters = []Parameter{{Name: "options", Value: "-c password=do-not-store"}}
	if _, err := ValidateDraft(draft, Document{}); err == nil {
		t.Fatal("advanced password unexpectedly accepted")
	}
}

func TestValidateDraftRejectsInvalidFields(t *testing.T) {
	draft := validDraft()
	draft.Name = ""
	draft.Host = "one,two"
	draft.Port = "65536"
	draft.Database = ""
	draft.User = ""
	draft.SSLMode = "sometimes"
	draft.AdvancedParameters = []Parameter{{Name: "bad name", Value: "x"}, {Name: "bad name", Value: "y"}}
	if _, err := ValidateDraft(draft, Document{}); err == nil {
		t.Fatal("invalid draft unexpectedly accepted")
	}
}

func TestValidateDraftRejectsTerminalControlCharacters(t *testing.T) {
	for field, mutate := range map[string]func(*Draft){
		"name":     func(draft *Draft) { draft.Name = "unsafe\x1b[31m" },
		"host":     func(draft *Draft) { draft.Host = "host\nname" },
		"database": func(draft *Draft) { draft.Database = "db\rname" },
		"user":     func(draft *Draft) { draft.User = "user\x00name" },
	} {
		t.Run(field, func(t *testing.T) {
			draft := validDraft()
			mutate(&draft)
			if _, err := ValidateDraft(draft, Document{}); err == nil {
				t.Fatalf("control character in %s unexpectedly accepted", field)
			}
		})
	}
}
