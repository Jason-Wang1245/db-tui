package app

import "testing"

func TestDiagnosticsIsBoundedAndReturnsCopies(t *testing.T) {
	diagnostics := NewDiagnostics(2)
	diagnostics.Record(Diagnostic{Operation: "one"})
	diagnostics.Record(Diagnostic{Operation: "two"})
	diagnostics.Record(Diagnostic{Operation: "three"})

	entries := diagnostics.Snapshot()
	if len(entries) != 2 || entries[0].Operation != "two" || entries[1].Operation != "three" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	entries[0].Operation = "mutated"
	if diagnostics.Snapshot()[0].Operation != "two" {
		t.Fatal("snapshot mutation changed stored diagnostics")
	}
}
