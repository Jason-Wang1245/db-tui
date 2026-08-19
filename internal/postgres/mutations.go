package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/grid"
)

var _ grid.MutationApplier = (*Session)(nil)

func (session *Session) Apply(ctx context.Context, request grid.ApplyRequest) (grid.ApplyResult, error) {
	if err := validateApplyRequest(request); err != nil {
		return grid.ApplyResult{}, err
	}
	tx, err := session.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return grid.ApplyResult{}, ClassifyError("begin staged changes", err)
	}
	defer func() {
		rollbackContext, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		_ = tx.Rollback(rollbackContext)
	}()

	result := grid.ApplyResult{}
	for index, mutation := range request.Mutations {
		var mutationErr error
		switch mutation.Kind {
		case grid.MutationDelete:
			mutationErr = applyDelete(ctx, tx, request.Relation, mutation)
			if mutationErr == nil {
				result.Deleted++
			}
		case grid.MutationUpdate:
			mutationErr = applyUpdate(ctx, tx, request.Relation, mutation)
			if mutationErr == nil {
				result.Updated++
			}
		case grid.MutationInsert:
			mutationErr = applyInsert(ctx, tx, request.Relation, mutation)
			if mutationErr == nil {
				result.Inserted++
			}
		default:
			mutationErr = core.NewError("apply staged changes", core.ErrorValidation, "The staged change kind is invalid.", false, nil)
		}
		if mutationErr != nil {
			column := mutationErrorColumn(request.Relation, mutation, mutationErr)
			current := conflictCurrentRow(ctx, tx, request.Relation, mutation, mutationErr)
			return grid.ApplyResult{}, &grid.MutationError{
				Mutation: index, Column: column, Current: current, Err: classifyMutationError(mutationErr),
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return grid.ApplyResult{}, classifyApplyCommitError(err)
	}
	return result, nil
}

func classifyApplyCommitError(err error) error {
	if errors.Is(err, pgx.ErrTxCommitRollback) {
		return core.NewError(
			"commit staged changes", core.ErrorConflict,
			"PostgreSQL rolled back the complete staged batch during commit.", true, err,
		)
	}
	return &grid.ApplyUncertainError{Err: core.NewError(
		"commit staged changes", core.ErrorNetwork,
		"The connection was lost while confirming commit. Do not retry these staged changes until you reconnect and reload the table.", false, err,
	)}
}

func validateApplyRequest(request grid.ApplyRequest) error {
	relation := request.Relation
	if relation.ID == (grid.RelationID{}) || len(request.Mutations) == 0 {
		return core.NewError("apply staged changes", core.ErrorValidation, "The staged change set is empty or missing relation metadata.", false, nil)
	}
	if relation.Kind != grid.RelationTable && relation.Kind != grid.RelationPartitionedTable {
		return core.NewError("apply staged changes", core.ErrorUnsupported, "Grid changes are limited to ordinary and partitioned tables.", false, nil)
	}
	if len(relation.Identity) == 0 || !relation.HasXMin {
		return core.NewError("apply staged changes", core.ErrorUnsupported, "This table has no safe identity and xmin concurrency token.", false, nil)
	}
	return nil
}

func applyInsert(ctx context.Context, tx pgx.Tx, relation grid.Relation, mutation grid.Mutation) error {
	if !relation.CanInsert {
		return core.NewError("insert row", core.ErrorPermission, "INSERT is unavailable for this table or role.", false, nil)
	}
	columns := orderedMutationColumns(relation, mutation.Values)
	if len(columns) == 0 {
		_, err := tx.Exec(ctx, "insert into "+quotedRelation(relation.ID)+" default values")
		return err
	}
	names := make([]string, 0, len(columns))
	expressions := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		if !column.CanInsert || column.Generated || column.Identity {
			return core.NewError("insert row", core.ErrorUnsupported, column.Name+" is not insertable.", false, nil)
		}
		names = append(names, quotedColumn(column.Name))
		value := mutation.Values[column.Name]
		expression, nextArgs := mutationExpression(value, len(args)+1)
		expressions = append(expressions, expression)
		args = append(args, nextArgs...)
	}
	query := "insert into " + quotedRelation(relation.ID) + " (" + strings.Join(names, ", ") + ") values (" + strings.Join(expressions, ", ") + ")"
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return core.NewError("insert row", core.ErrorConflict, "PostgreSQL did not insert the staged row.", false, nil)
	}
	return nil
}

func applyUpdate(ctx context.Context, tx pgx.Tx, relation grid.Relation, mutation grid.Mutation) error {
	if !relation.CanUpdate {
		return core.NewError("update row", core.ErrorPermission, "UPDATE is unavailable for this table or role.", false, nil)
	}
	columns := orderedMutationColumns(relation, mutation.Values)
	if len(columns) == 0 {
		return core.NewError("update row", core.ErrorValidation, "The staged update has no changed columns.", false, nil)
	}
	assignments := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns)+len(relation.Identity)+1)
	for _, column := range columns {
		if !column.CanUpdate || column.Generated || column.Identity {
			return core.NewError("update row", core.ErrorUnsupported, column.Name+" is not updatable.", false, nil)
		}
		expression, nextArgs := mutationExpression(mutation.Values[column.Name], len(args)+1)
		assignments = append(assignments, quotedColumn(column.Name)+" = "+expression)
		args = append(args, nextArgs...)
	}
	predicate, identityArgs, err := optimisticPredicate(relation, mutation.Original, len(args)+1)
	if err != nil {
		return err
	}
	args = append(args, identityArgs...)
	tag, err := tx.Exec(ctx, "update "+quotedRelation(relation.ID)+" set "+strings.Join(assignments, ", ")+" where "+predicate, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return conflictError("update row")
	}
	if tag.RowsAffected() != 1 {
		return core.NewError("update row", core.ErrorConflict, "The staged update matched more than one row; the transaction was rolled back.", false, nil)
	}
	return nil
}

func applyDelete(ctx context.Context, tx pgx.Tx, relation grid.Relation, mutation grid.Mutation) error {
	if !relation.CanDelete {
		return core.NewError("delete row", core.ErrorPermission, "DELETE is unavailable for this table or role.", false, nil)
	}
	predicate, args, err := optimisticPredicate(relation, mutation.Original, 1)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, "delete from "+quotedRelation(relation.ID)+" where "+predicate, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return conflictError("delete row")
	}
	if tag.RowsAffected() != 1 {
		return core.NewError("delete row", core.ErrorConflict, "The staged delete matched more than one row; the transaction was rolled back.", false, nil)
	}
	return nil
}

func mutationExpression(value grid.StagedValue, parameter int) (string, []any) {
	switch value.Kind {
	case grid.ValueDefault:
		return "default", nil
	case grid.ValueNull:
		return fmt.Sprintf("$%d", parameter), []any{nil}
	default:
		return fmt.Sprintf("$%d", parameter), []any{value.Text}
	}
}

func optimisticPredicate(relation grid.Relation, original grid.Row, firstParameter int) (string, []any, error) {
	conditions := make([]string, 0, len(relation.Identity)+1)
	args := make([]any, 0, len(relation.Identity)+1)
	for _, name := range relation.Identity {
		value, ok := original.Identity[name]
		if !ok {
			return "", nil, core.NewError("apply staged changes", core.ErrorValidation, "A staged row has an incomplete original identity.", false, nil)
		}
		conditions = append(conditions, fmt.Sprintf("%s is not distinct from $%d", quotedColumn(name), firstParameter+len(args)))
		args = append(args, value)
	}
	conditions = append(conditions, fmt.Sprintf("xmin::text::bigint = $%d", firstParameter+len(args)))
	args = append(args, int64(original.XMin))
	return strings.Join(conditions, " and "), args, nil
}

func orderedMutationColumns(relation grid.Relation, values map[string]grid.StagedValue) []grid.Column {
	columns := make([]grid.Column, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, column := range relation.Columns {
		if _, ok := values[column.Name]; ok {
			columns = append(columns, column)
			seen[column.Name] = true
		}
	}
	if len(seen) != len(values) {
		unknown := make([]string, 0)
		for name := range values {
			if !seen[name] {
				unknown = append(unknown, name)
			}
		}
		sort.Strings(unknown)
		for _, name := range unknown {
			columns = append(columns, grid.Column{Name: name})
		}
	}
	return columns
}

func mutationErrorColumn(relation grid.Relation, mutation grid.Mutation, err error) string {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.ColumnName != "" {
		for _, column := range relation.Columns {
			if column.Name == postgresError.ColumnName {
				return column.Name
			}
		}
	}
	if len(mutation.Values) != 1 {
		return ""
	}
	for column := range mutation.Values {
		return column
	}
	return ""
}

func conflictError(operation string) *core.Error {
	return core.NewError(operation, core.ErrorConflict, "The row changed or was deleted after it was loaded; the complete batch was rolled back.", true, nil)
}

func classifyMutationError(err error) error {
	var structured *core.Error
	if errors.As(err, &structured) {
		return structured
	}
	return ClassifyError("apply staged changes", err)
}

func conflictCurrentRow(ctx context.Context, tx pgx.Tx, relation grid.Relation, mutation grid.Mutation, err error) *grid.Row {
	var structured *core.Error
	if mutation.Kind == grid.MutationInsert || !errors.As(err, &structured) || structured.Category != core.ErrorConflict {
		return nil
	}
	conditions := make([]string, 0, len(relation.Identity))
	args := make([]any, 0, len(relation.Identity))
	for _, column := range relation.Identity {
		value, ok := mutation.Original.Identity[column]
		if !ok {
			return nil
		}
		conditions = append(conditions, fmt.Sprintf("%s is not distinct from $%d", quotedColumn(column), len(args)+1))
		args = append(args, value)
	}
	rows, queryErr := tx.Query(ctx, baseProjection(relation, nil)+" where "+strings.Join(conditions, " and ")+" limit 1", args...)
	if queryErr != nil {
		return nil
	}
	fetched, queryErr := collectFetchedRows(rows, relation, nil)
	if queryErr != nil || len(fetched) == 0 {
		return nil
	}
	current := fetched[0].row
	return &current
}
