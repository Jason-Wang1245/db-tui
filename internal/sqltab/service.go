// Package sqltab owns SQL editor, run, and result state.
package sqltab

import (
	"context"

	"charm.land/bubbles/v2/textarea"

	"github.com/Jason-Wang1245/db-tui/internal/core"
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

type Output struct {
	Kind         OutputKind
	CommandTag   string
	AffectedRows int64
	Columns      []string
	Rows         [][]any
	Truncated    bool
	Incomplete   bool
}

type RunResult struct {
	Outputs []Output
}

type SQLExecutor interface {
	Execute(context.Context, RunRequest) (RunResult, error)
}

type State struct {
	Editor   textarea.Model
	LastRun  RunRequest
	Result   RunResult
	Running  bool
	LastMeta core.RequestMeta
}

func NewState() State {
	editor := textarea.New()
	editor.SetWidth(80)
	editor.SetHeight(10)
	return State{Editor: editor}
}
