// Package workspace owns catalog navigation and the mixed-tab shell.
package workspace

import (
	"context"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

type RelationKind string

const (
	RelationTable            RelationKind = "table"
	RelationPartitionedTable RelationKind = "partitioned_table"
	RelationView             RelationKind = "view"
	RelationMaterializedView RelationKind = "materialized_view"
	RelationForeignTable     RelationKind = "foreign_table"
)

type Relation struct {
	Schema string
	Name   string
	Kind   RelationKind
}

type CatalogReader interface {
	Relations(context.Context) ([]Relation, error)
}

type TabKind string

const (
	TabTable TabKind = "table"
	TabSQL   TabKind = "sql"
)

type TabEnvelope struct {
	ID            core.TabID
	Title         string
	Kind          TabKind
	LastFocus     string
	ActiveRequest core.RequestMeta
}
