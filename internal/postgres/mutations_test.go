package postgres

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/grid"
)

func mutationRelationFixture() grid.Relation {
	return grid.Relation{
		ID:   grid.RelationID{Schema: `odd"schema`, Name: `records; safe`},
		Kind: grid.RelationTable, Identity: []string{"tenant_id", "id"}, HasXMin: true,
		CanInsert: true, CanUpdate: true, CanDelete: true,
		Columns: []grid.Column{
			{Name: "tenant_id", CanInsert: true, CanUpdate: true},
			{Name: "id", CanInsert: true, CanUpdate: true},
			{Name: "payload", CanInsert: true, CanUpdate: true},
		},
	}
}

func TestMutationExpressionsKeepEmptyNullAndDefaultDistinct(t *testing.T) {
	expression, args := mutationExpression(grid.StagedValue{Kind: grid.ValueText, Text: ""}, 2)
	if expression != "$2" || len(args) != 1 || args[0] != "" {
		t.Fatalf("empty text expression=%q args=%#v", expression, args)
	}
	expression, args = mutationExpression(grid.StagedValue{Kind: grid.ValueNull}, 3)
	if expression != "$3" || len(args) != 1 || args[0] != nil {
		t.Fatalf("NULL expression=%q args=%#v", expression, args)
	}
	expression, args = mutationExpression(grid.StagedValue{Kind: grid.ValueDefault}, 4)
	if expression != "default" || len(args) != 0 {
		t.Fatalf("DEFAULT expression=%q args=%#v", expression, args)
	}
}

func TestOptimisticPredicateQuotesIdentityAndIncludesOriginalXMin(t *testing.T) {
	relation := mutationRelationFixture()
	relation.Identity = []string{`tenant"id`, "id"}
	row := grid.Row{Identity: map[string]any{`tenant"id`: int64(7), "id": int64(9)}, XMin: 42}
	predicate, args, err := optimisticPredicate(relation, row, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"tenant""id" is not distinct from $3`, `"id" is not distinct from $4`, `xmin::text::bigint = $5`} {
		if !strings.Contains(predicate, expected) {
			t.Fatalf("predicate missing %q: %s", expected, predicate)
		}
	}
	if len(args) != 3 || args[0] != int64(7) || args[1] != int64(9) || args[2] != int64(42) {
		t.Fatalf("optimistic args = %#v", args)
	}
}

func TestApplyValidationRejectsUnsafeRelationMetadata(t *testing.T) {
	relation := mutationRelationFixture()
	relation.Identity = nil
	err := validateApplyRequest(grid.ApplyRequest{Relation: relation, Mutations: []grid.Mutation{{Kind: grid.MutationInsert}}})
	if err == nil || !strings.Contains(err.Error(), "safe identity") {
		t.Fatalf("unsafe relation error = %v", err)
	}
}

func TestMutationErrorColumnPrefersPostgreSQLColumnMetadata(t *testing.T) {
	relation := mutationRelationFixture()
	mutation := grid.Mutation{Values: map[string]grid.StagedValue{
		"tenant_id": {Kind: grid.ValueText, Text: "1"},
		"payload":   {Kind: grid.ValueText, Text: "bad"},
	}}
	if got := mutationErrorColumn(relation, mutation, &pgconn.PgError{ColumnName: "payload"}); got != "payload" {
		t.Fatalf("mutation error column = %q", got)
	}
}

func TestCommitErrorsDistinguishKnownRollbackFromUncertainOutcome(t *testing.T) {
	known := classifyApplyCommitError(pgx.ErrTxCommitRollback)
	var knownCore *core.Error
	if !errors.As(known, &knownCore) || knownCore.Category != core.ErrorConflict || !knownCore.Retryable {
		t.Fatalf("known rollback = %#v", known)
	}
	uncertain := classifyApplyCommitError(errors.New("connection lost"))
	var uncertainOutcome *grid.ApplyUncertainError
	var uncertainCore *core.Error
	if !errors.As(uncertain, &uncertainOutcome) || !errors.As(uncertain, &uncertainCore) || uncertainCore.Retryable {
		t.Fatalf("uncertain commit = %#v core=%#v", uncertain, uncertainCore)
	}
}
