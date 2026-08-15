package postgres

import (
	"testing"

	"github.com/Jason-Wang1245/db-tui/internal/workspace"
)

func TestRelationKindMapsPostgreSQLRelkind(t *testing.T) {
	tests := map[string]workspace.RelationKind{
		"r": workspace.RelationTable,
		"p": workspace.RelationPartitionedTable,
		"v": workspace.RelationView,
		"m": workspace.RelationMaterializedView,
		"f": workspace.RelationForeignTable,
	}
	for input, expected := range tests {
		if actual := relationKind(input); actual != expected {
			t.Errorf("relationKind(%q) = %q, want %q", input, actual, expected)
		}
	}
}
