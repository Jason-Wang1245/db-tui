package core

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorKeepsCauseWithoutDisplayingIt(t *testing.T) {
	cause := errors.New("postgres://alice:secret@example.test/app")
	err := NewError("connect", ErrorAuthentication, "authentication failed", false, cause)

	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("safe error leaked its cause: %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause is not available to explicit diagnostics")
	}
}
