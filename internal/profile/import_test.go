package profile

import (
	"strings"
	"testing"
)

func TestImportURLExtractsPasswordAndCanonicalFields(t *testing.T) {
	result, err := ImportURL("postgresql://alice:s%40cret@db.example.com:6543/my%20db?sslmode=require&search_path=private%2Cpublic")
	if err != nil {
		t.Fatal(err)
	}
	if result.Password != "s@cret" || result.Draft.Password != "s@cret" || !result.Draft.ReplacePassword {
		t.Fatalf("password extraction = %#v", result)
	}
	if result.Draft.Host != "db.example.com" || result.Draft.Port != "6543" || result.Draft.Database != "my db" || result.Draft.User != "alice" {
		t.Fatalf("draft = %#v", result.Draft)
	}
	if result.Draft.SSLMode != "require" || len(result.Draft.AdvancedParameters) != 1 || result.Draft.AdvancedParameters[0].Name != "search_path" {
		t.Fatalf("parameters = %#v", result.Draft.AdvancedParameters)
	}
}

func TestImportURLPreservesExplicitEmptyPassword(t *testing.T) {
	result, err := ImportURL("postgresql://alice:@db.example.com/app")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Draft.ReplacePassword || result.Draft.Password != "" {
		t.Fatalf("empty password was not marked explicit: replace=%v", result.Draft.ReplacePassword)
	}
}

func TestImportURLNeverEchoesPasswordInErrors(t *testing.T) {
	secret := "never-show-this"
	_, err := ImportURL("postgresql://alice:" + secret + "@db.example.com:bad/app")
	if err == nil {
		t.Fatal("malformed URL unexpectedly accepted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked password: %v", err)
	}
}

func TestImportURLRejectsUnsupportedOrUnsafeForms(t *testing.T) {
	cases := []string{
		"mysql://alice:x@db.example.com/app",
		"postgresql://alice:x@one.example.com,two.example.com/app",
		"postgresql://alice:x@db.example.com/app?password=second",
		"postgresql://alice:x@db.example.com/app?sslmode=require&SSLMODE=disable",
		"postgresql://alice:x@db.example.com/one/two",
		"postgresql://alice:x@db.example.com/app?sslmode=invalid",
	}
	for _, raw := range cases {
		if _, err := ImportURL(raw); err == nil {
			t.Errorf("URL %q unexpectedly accepted", raw)
		}
	}
}
