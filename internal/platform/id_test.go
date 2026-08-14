package platform

import (
	"regexp"
	"testing"
)

func TestRandomIDGeneratorCreatesRFC4122UUIDs(t *testing.T) {
	id, err := (RandomIDGenerator{}).NewID()
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(string(id)) {
		t.Fatalf("ID %q is not an RFC 4122 version 4 UUID", id)
	}
}
