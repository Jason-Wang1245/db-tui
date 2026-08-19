// Package sqltab owns SQL editor, run, and result state.
package sqltab

import (
	"context"
	"time"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

const (
	ResultPageSize       = 100
	MaxRowsPerResult     = 10_000
	MaxCapturedDataBytes = 64 << 20
)

type OutputKind string

const (
	OutputRows    OutputKind = "rows"
	OutputCommand OutputKind = "command"
	OutputError   OutputKind = "error"
)

type RunRequest struct {
	Meta     core.RequestMeta
	Snapshot string
}

type Column struct {
	Name     string
	DataType string
	TypeOID  uint32
}

type Cell struct {
	Raw     any
	Display string
	Full    string
	Null    bool
}

type Notice struct {
	Severity string
	Message  string
	SQLState string
	Detail   string
	Hint     string
}

type Output struct {
	Kind         OutputKind
	CommandTag   string
	AffectedRows int64
	Columns      []Column
	Rows         [][]Cell
	Truncated    bool
	Incomplete   bool
	Duration     time.Duration
	Error        *core.Error
}

type RunResult struct {
	Outputs       []Output
	Notices       []Notice
	Duration      time.Duration
	CapturedBytes int64
	Warning       string
}

type SQLExecutor interface {
	Execute(context.Context, RunRequest) (RunResult, error)
}
