// Package grid owns deterministic record browsing and staged mutation state.
package grid

import (
	"context"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

const DefaultPageSize = 100

type RelationID struct {
	Schema string
	Name   string
}

type RelationKind string

const (
	RelationTable            RelationKind = "table"
	RelationPartitionedTable RelationKind = "partitioned_table"
	RelationView             RelationKind = "view"
	RelationMaterializedView RelationKind = "materialized_view"
	RelationForeignTable     RelationKind = "foreign_table"
)

type Column struct {
	Name         string
	DataType     string
	TypeOID      uint32
	Nullable     bool
	HasDefault   bool
	Generated    bool
	Identity     bool
	CanSelect    bool
	Sortable     bool
	IdentityPart bool
}

type Relation struct {
	ID              RelationID
	Kind            RelationKind
	Columns         []Column
	Identity        []string
	IdentityPrimary bool
	CanSelect       bool
	HasXMin         bool
	BestEffort      bool
	ReadOnlyReason  string
}

type Sort struct {
	Column    string
	Ascending bool
}

type PageDirection string

const (
	PageFirst    PageDirection = "first"
	PageNext     PageDirection = "next"
	PagePrevious PageDirection = "previous"
)

type PageRequest struct {
	Relation  RelationID
	Cursor    string
	Direction PageDirection
	Sort      Sort
	Limit     int
}

type Cell struct {
	Raw     any
	Display string
	Null    bool
}

type Row struct {
	Identity map[string]any
	XMin     uint32
	Cells    []Cell
}

type Page struct {
	Rows       []Row
	NextCursor string
	PrevCursor string
	BestEffort bool
}

type TableBrowser interface {
	Describe(context.Context, RelationID) (Relation, error)
	FetchPage(context.Context, Relation, PageRequest) (Page, error)
	FetchCurrentRow(context.Context, RelationID, map[string]any) (Row, error)
}

type MutationKind string

const (
	MutationInsert MutationKind = "insert"
	MutationUpdate MutationKind = "update"
	MutationDelete MutationKind = "delete"
)

type Mutation struct {
	Kind     MutationKind
	Original Row
	Staged   Row
	Request  core.RequestMeta
}

type ApplyRequest struct {
	Relation  RelationID
	Mutations []Mutation
}

type ApplyResult struct {
	Inserted int
	Updated  int
	Deleted  int
}

type MutationApplier interface {
	Apply(context.Context, ApplyRequest) (ApplyResult, error)
}
