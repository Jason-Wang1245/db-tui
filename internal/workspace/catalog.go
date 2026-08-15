// Package workspace owns catalog navigation and the mixed-tab shell.
package workspace

import (
	"context"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

type Schema struct {
	Name string
}

type RelationKind string

const (
	RelationTable            RelationKind = "table"
	RelationPartitionedTable RelationKind = "partitioned_table"
	RelationView             RelationKind = "view"
	RelationMaterializedView RelationKind = "materialized_view"
	RelationForeignTable     RelationKind = "foreign_table"
)

type Relation struct {
	Schema    string
	Name      string
	Kind      RelationKind
	CanSelect bool
}

// CatalogReader is intentionally lazy: the shell loads schema names once and
// requests a schema's relations only when that schema is expanded.
type CatalogReader interface {
	Schemas(context.Context) ([]Schema, error)
	Relations(context.Context, string) ([]Relation, error)
}

type TabKind string

const (
	TabTable TabKind = "table"
	TabSQL   TabKind = "sql"
)

type TabLifecycle string

const (
	TabIdle    TabLifecycle = "idle"
	TabRunning TabLifecycle = "running"
	TabFailed  TabLifecycle = "failed"
)

type TabEnvelope struct {
	ID            core.TabID
	Title         string
	Kind          TabKind
	Lifecycle     TabLifecycle
	Dirty         bool
	LastFocus     Focus
	ActiveRequest core.RequestMeta
}

type TableTabState struct {
	Relation Relation
}

type SQLTabState struct{}

type Tab struct {
	Envelope TabEnvelope
	Table    *TableTabState
	SQL      *SQLTabState
}
