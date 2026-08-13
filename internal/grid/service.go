// Package grid owns record browsing and staged mutation state.
package grid

import (
	"context"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

type RelationID struct {
	Schema string
	Name   string
}

type Sort struct {
	Column    string
	Ascending bool
}

type PageRequest struct {
	Relation RelationID
	Cursor   string
	Sort     Sort
	Limit    int
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
	FetchPage(context.Context, PageRequest) (Page, error)
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
