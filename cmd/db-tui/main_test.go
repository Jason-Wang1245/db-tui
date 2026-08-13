package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionDoesNotStartTUI(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "db-tui "+version) {
		t.Fatalf("version output %q does not contain version", got)
	}
}

func TestHelpDoesNotStartTUI(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run returned %d", code)
	}
	if got := stderr.String(); !strings.Contains(got, "Usage: db-tui") {
		t.Fatalf("help output %q does not contain usage", got)
	}
}

func TestUnexpectedArgumentsFail(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"unexpected"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run returned %d, want 2", code)
	}
}
