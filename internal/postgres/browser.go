package postgres

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/grid"
)

var _ grid.TableBrowser = (*Session)(nil)

const relationMetadataQuery = `
select
  c.oid,
  c.relkind::text,
  pg_catalog.has_table_privilege(c.oid, 'SELECT')
    or pg_catalog.has_any_column_privilege(c.oid, 'SELECT'),
  pg_catalog.has_table_privilege(c.oid, 'INSERT')
    or pg_catalog.has_any_column_privilege(c.oid, 'INSERT'),
  pg_catalog.has_table_privilege(c.oid, 'UPDATE')
    or pg_catalog.has_any_column_privilege(c.oid, 'UPDATE'),
  pg_catalog.has_table_privilege(c.oid, 'DELETE')
from pg_catalog.pg_class c
join pg_catalog.pg_namespace n on n.oid = c.relnamespace
where n.nspname = $1
  and c.relname = $2
  and c.relkind in ('r', 'p', 'v', 'm', 'f')`

const columnMetadataQuery = `
select
  a.attname,
  pg_catalog.format_type(a.atttypid, a.atttypmod),
  a.atttypid,
  not a.attnotnull,
  a.atthasdef,
  a.attgenerated <> '',
  a.attidentity <> '',
  pg_catalog.has_column_privilege($1::oid, a.attnum, 'SELECT'),
  pg_catalog.has_column_privilege($1::oid, a.attnum, 'INSERT'),
  pg_catalog.has_column_privilege($1::oid, a.attnum, 'UPDATE'),
  exists (
    select 1
    from pg_catalog.pg_operator o
    where o.oprleft = a.atttypid
      and o.oprright = a.atttypid
      and o.oprname in ('<', '>')
  )
from pg_catalog.pg_attribute a
where a.attrelid = $1::oid
  and a.attnum > 0
  and not a.attisdropped
order by a.attnum`

const identityMetadataQuery = `
select
  i.indisprimary,
  array_agg(a.attname order by key.ord)::text[]
from pg_catalog.pg_index i
join pg_catalog.pg_class index_relation on index_relation.oid = i.indexrelid
cross join lateral unnest(i.indkey::smallint[]) with ordinality as key(attnum, ord)
join pg_catalog.pg_attribute a
  on a.attrelid = i.indrelid
 and a.attnum = key.attnum
where i.indrelid = $1::oid
  and (i.indisprimary or i.indisunique)
  and i.indisvalid
  and i.indpred is null
  and i.indexprs is null
  and key.ord <= i.indnkeyatts
group by i.indexrelid, i.indisprimary, i.indnkeyatts, index_relation.relname
having count(*) = i.indnkeyatts
   and bool_and(a.attnotnull)
order by i.indisprimary desc, i.indnkeyatts, index_relation.relname
limit 1`

func (session *Session) Describe(ctx context.Context, relationID grid.RelationID) (grid.Relation, error) {
	var oid uint32
	var rawKind string
	var canSelect, canInsert, canUpdate, canDelete bool
	err := session.pool.QueryRow(ctx, relationMetadataQuery, relationID.Schema, relationID.Name).Scan(
		&oid, &rawKind, &canSelect, &canInsert, &canUpdate, &canDelete,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return grid.Relation{}, core.NewError(
			"describe relation", core.ErrorValidation,
			"The relation no longer exists or is not a supported table or view.", false, err,
		)
	}
	if err != nil {
		return grid.Relation{}, ClassifyError("describe relation", err)
	}

	relation := grid.Relation{
		ID: relationID, Kind: gridRelationKind(rawKind), CanSelect: canSelect,
		HasXMin: rawKind == "r" || rawKind == "p",
	}
	rows, err := session.pool.Query(ctx, columnMetadataQuery, oid)
	if err != nil {
		return grid.Relation{}, ClassifyError("load column metadata", err)
	}
	columns, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (grid.Column, error) {
		var column grid.Column
		err := row.Scan(
			&column.Name, &column.DataType, &column.TypeOID, &column.Nullable,
			&column.HasDefault, &column.Generated, &column.Identity,
			&column.CanSelect, &column.CanInsert, &column.CanUpdate, &column.Sortable,
		)
		return column, err
	})
	if err != nil {
		return grid.Relation{}, ClassifyError("load column metadata", err)
	}
	for index := range columns {
		if columns[index].Generated || columns[index].Identity {
			columns[index].CanInsert = false
			columns[index].CanUpdate = false
		}
	}
	relation.MutationColumns = append([]grid.Column(nil), columns...)
	for _, column := range relation.MutationColumns {
		if column.CanSelect {
			relation.Columns = append(relation.Columns, column)
		}
	}
	if !canSelect || len(relation.Columns) == 0 {
		return grid.Relation{}, core.NewError(
			"describe relation", core.ErrorPermission,
			"The current role does not have SELECT privilege on any columns in this relation.", false, nil,
		)
	}

	var identityPrimary bool
	var identity []string
	err = session.pool.QueryRow(ctx, identityMetadataQuery, oid).Scan(&identityPrimary, &identity)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return grid.Relation{}, ClassifyError("load row identity", err)
	}
	readable := make(map[string]bool, len(relation.Columns))
	for _, column := range relation.Columns {
		readable[column.Name] = true
	}
	for _, name := range identity {
		if !readable[name] {
			identity = nil
			identityPrimary = false
			break
		}
	}
	relation.Identity = identity
	relation.IdentityPrimary = identityPrimary
	for index := range relation.Columns {
		relation.Columns[index].IdentityPart = slices.Contains(identity, relation.Columns[index].Name)
	}
	for index := range relation.MutationColumns {
		relation.MutationColumns[index].IdentityPart = slices.Contains(identity, relation.MutationColumns[index].Name)
	}

	if len(identity) == 0 {
		relation.BestEffort = true
		relation.ReadOnlyReason = "No usable primary or unique NOT NULL key; paging is best-effort and read-only."
	} else if relation.Kind != grid.RelationTable && relation.Kind != grid.RelationPartitionedTable {
		relation.ReadOnlyReason = "This relation type is browse-only."
	} else {
		relation.CanInsert = canInsert
		relation.CanUpdate = canUpdate
		relation.CanDelete = canDelete
		if !canInsert {
			relation.InsertReason = "The current role has no usable INSERT privilege."
		}
		if !canUpdate {
			relation.UpdateReason = "The current role has no usable UPDATE privilege."
		}
		if !canDelete {
			relation.DeleteReason = "The current role has no DELETE privilege."
		}
		for _, column := range relation.MutationColumns {
			if !column.Nullable && !column.HasDefault && !column.Generated && !column.Identity && (!column.CanInsert || !column.CanSelect) {
				relation.CanInsert = false
				relation.InsertReason = column.Name + " is required without a default but cannot be edited in this grid."
				break
			}
		}
		if relation.CanUpdate {
			relation.CanUpdate = slices.ContainsFunc(relation.Columns, func(column grid.Column) bool { return column.CanUpdate })
			if !relation.CanUpdate {
				relation.UpdateReason = "No readable column is updatable for the current role."
			}
		}
		if !relation.CanInsert && !relation.CanUpdate && !relation.CanDelete {
			relation.ReadOnlyReason = "The current role has no usable INSERT, UPDATE, or DELETE privileges."
		}
	}
	return relation, nil
}

func (session *Session) FetchPage(ctx context.Context, relation grid.Relation, request grid.PageRequest) (grid.Page, error) {
	if relation.ID != request.Relation {
		return grid.Page{}, core.NewError("fetch page", core.ErrorValidation, "The page request does not match its relation metadata.", false, nil)
	}
	if len(relation.Columns) == 0 {
		return grid.Page{}, core.NewError("fetch page", core.ErrorUnsupported, "The relation has no readable columns.", false, nil)
	}
	limit := request.Limit
	if limit <= 0 {
		limit = grid.DefaultPageSize
	}
	if limit > 500 {
		return grid.Page{}, core.NewError("fetch page", core.ErrorValidation, "The requested page size is too large.", false, nil)
	}
	if request.Direction == "" {
		request.Direction = grid.PageFirst
	}
	if request.Direction != grid.PageFirst && request.Direction != grid.PageNext && request.Direction != grid.PagePrevious {
		return grid.Page{}, core.NewError("fetch page", core.ErrorValidation, "The page direction is invalid.", false, nil)
	}
	if len(relation.Identity) == 0 {
		return session.fetchOffsetPage(ctx, relation, request, limit)
	}
	return session.fetchKeysetPage(ctx, relation, request, limit)
}

type orderTerm struct {
	Column    string
	Ascending bool
}

type cursorValue struct {
	Null bool   `json:"null,omitempty"`
	Text string `json:"text,omitempty"`
}

type pageCursor struct {
	Version  int             `json:"version"`
	Relation grid.RelationID `json:"relation"`
	Sort     grid.Sort       `json:"sort"`
	Terms    []string        `json:"terms,omitempty"`
	Values   []cursorValue   `json:"values,omitempty"`
	Offset   int             `json:"offset,omitempty"`
}

type fetchedRow struct {
	row    grid.Row
	cursor []cursorValue
}

func (session *Session) fetchKeysetPage(ctx context.Context, relation grid.Relation, request grid.PageRequest, limit int) (grid.Page, error) {
	terms, err := effectiveOrder(relation, request.Sort)
	if err != nil {
		return grid.Page{}, err
	}
	cursor := pageCursor{Version: 1, Relation: relation.ID, Sort: request.Sort}
	if request.Direction != grid.PageFirst {
		cursor, err = decodeCursor(request.Cursor)
		if err != nil {
			return grid.Page{}, err
		}
		if err := validateKeysetCursor(cursor, relation.ID, request.Sort, terms); err != nil {
			return grid.Page{}, err
		}
	}
	query, args := keysetQuery(relation, terms, cursor.Values, request.Direction, limit+1)
	rows, err := session.pool.Query(ctx, query, args...)
	if err != nil {
		return grid.Page{}, ClassifyError("fetch table page", err)
	}
	fetched, err := collectFetchedRows(rows, relation, terms)
	if err != nil {
		return grid.Page{}, ClassifyError("read table page", err)
	}

	extra := len(fetched) > limit
	if extra {
		fetched = fetched[:limit]
	}
	if request.Direction == grid.PagePrevious {
		slices.Reverse(fetched)
	}
	page := grid.Page{Rows: rowsOnly(fetched)}
	hasPrevious := request.Direction == grid.PageNext || (request.Direction == grid.PagePrevious && extra)
	hasNext := (request.Direction != grid.PagePrevious && extra) || request.Direction == grid.PagePrevious
	termNames := make([]string, len(terms))
	for index, term := range terms {
		termNames[index] = term.Column
	}
	if len(fetched) > 0 && hasPrevious {
		page.PrevCursor, err = encodeCursor(pageCursor{
			Version: 1, Relation: relation.ID, Sort: request.Sort, Terms: termNames, Values: fetched[0].cursor,
		})
		if err != nil {
			return grid.Page{}, err
		}
	}
	if len(fetched) > 0 && hasNext {
		page.NextCursor, err = encodeCursor(pageCursor{
			Version: 1, Relation: relation.ID, Sort: request.Sort, Terms: termNames, Values: fetched[len(fetched)-1].cursor,
		})
		if err != nil {
			return grid.Page{}, err
		}
	}
	return page, nil
}

func (session *Session) fetchOffsetPage(ctx context.Context, relation grid.Relation, request grid.PageRequest, limit int) (grid.Page, error) {
	offset := 0
	if request.Direction != grid.PageFirst {
		cursor, err := decodeCursor(request.Cursor)
		if err != nil {
			return grid.Page{}, err
		}
		if cursor.Relation != relation.ID || cursor.Sort != request.Sort || cursor.Offset < 0 {
			return grid.Page{}, invalidCursorError()
		}
		offset = cursor.Offset
	}
	order, err := bestEffortOrder(relation, request.Sort)
	if err != nil {
		return grid.Page{}, err
	}
	query := baseProjection(relation, nil) + " order by " + order + fmt.Sprintf(" limit %d offset %d", limit+1, offset)
	rows, err := session.pool.Query(ctx, query)
	if err != nil {
		return grid.Page{}, ClassifyError("fetch best-effort table page", err)
	}
	fetched, err := collectFetchedRows(rows, relation, nil)
	if err != nil {
		return grid.Page{}, ClassifyError("read best-effort table page", err)
	}
	hasNext := len(fetched) > limit
	if hasNext {
		fetched = fetched[:limit]
	}
	page := grid.Page{Rows: rowsOnly(fetched), BestEffort: true}
	if offset > 0 {
		page.PrevCursor, err = encodeCursor(pageCursor{
			Version: 1, Relation: relation.ID, Sort: request.Sort, Offset: max(0, offset-limit),
		})
		if err != nil {
			return grid.Page{}, err
		}
	}
	if hasNext {
		page.NextCursor, err = encodeCursor(pageCursor{
			Version: 1, Relation: relation.ID, Sort: request.Sort, Offset: offset + limit,
		})
		if err != nil {
			return grid.Page{}, err
		}
	}
	return page, nil
}

func effectiveOrder(relation grid.Relation, sort grid.Sort) ([]orderTerm, error) {
	terms := make([]orderTerm, 0, len(relation.Identity)+1)
	if sort.Column != "" {
		column, ok := relationColumn(relation, sort.Column)
		if !ok || !column.Sortable {
			return nil, core.NewError("sort table", core.ErrorUnsupported, "The selected column cannot be sorted by PostgreSQL.", false, nil)
		}
		terms = append(terms, orderTerm{Column: sort.Column, Ascending: sort.Ascending})
	}
	for _, identity := range relation.Identity {
		if identity != sort.Column {
			terms = append(terms, orderTerm{Column: identity, Ascending: true})
		}
	}
	return terms, nil
}

func bestEffortOrder(relation grid.Relation, sort grid.Sort) (string, error) {
	parts := make([]string, 0, len(relation.Columns)+1)
	if sort.Column != "" {
		column, ok := relationColumn(relation, sort.Column)
		if !ok || !column.Sortable {
			return "", core.NewError("sort table", core.ErrorUnsupported, "The selected column cannot be sorted by PostgreSQL.", false, nil)
		}
		parts = append(parts, quotedColumn(column.Name)+orderSuffix(sort.Ascending, false))
	}
	for _, column := range relation.Columns {
		if column.Name == sort.Column {
			continue
		}
		parts = append(parts, "("+quotedColumn(column.Name)+"::text) asc nulls last")
	}
	if len(parts) == 0 {
		return "1", nil
	}
	return strings.Join(parts, ", "), nil
}

func keysetQuery(relation grid.Relation, terms []orderTerm, values []cursorValue, direction grid.PageDirection, limit int) (string, []any) {
	query := baseProjection(relation, terms)
	args := make([]any, len(values))
	for index, value := range values {
		if !value.Null {
			args[index] = value.Text
		}
	}
	if len(values) > 0 {
		query += " where " + seekPredicate(terms, values, direction)
	}
	order := make([]string, len(terms))
	for index, term := range terms {
		reverse := direction == grid.PagePrevious
		order[index] = quotedColumn(term.Column) + orderSuffix(term.Ascending, reverse)
	}
	query += " order by " + strings.Join(order, ", ") + fmt.Sprintf(" limit %d", limit)
	return query, args
}

func baseProjection(relation grid.Relation, cursorTerms []orderTerm) string {
	projections := make([]string, 0, len(relation.Columns)+len(cursorTerms)+1)
	for _, column := range relation.Columns {
		projections = append(projections, quotedColumn(column.Name))
	}
	if relation.HasXMin {
		projections = append(projections, "xmin::text::bigint")
	}
	for index, term := range cursorTerms {
		projections = append(projections, fmt.Sprintf(
			"%s::text as %s",
			quotedColumn(term.Column),
			quotedColumn(fmt.Sprintf("__dbtui_cursor_%d", index)),
		))
	}
	return "select " + strings.Join(projections, ", ") + " from " + quotedRelation(relation.ID)
}

func seekPredicate(terms []orderTerm, values []cursorValue, direction grid.PageDirection) string {
	before := direction == grid.PagePrevious
	groups := make([]string, 0, len(terms))
	for index, term := range terms {
		conditions := make([]string, 0, index+1)
		for prefix := 0; prefix < index; prefix++ {
			conditions = append(conditions, fmt.Sprintf("%s is not distinct from $%d", quotedColumn(terms[prefix].Column), prefix+1))
		}
		comparison := cursorComparison(term, values[index], index+1, before)
		if comparison == "false" {
			continue
		}
		conditions = append(conditions, comparison)
		groups = append(groups, "("+strings.Join(conditions, " and ")+")")
	}
	if len(groups) == 0 {
		return "false"
	}
	return "(" + strings.Join(groups, " or ") + ")"
}

func cursorComparison(term orderTerm, value cursorValue, parameter int, before bool) string {
	column := quotedColumn(term.Column)
	if term.Ascending {
		if before {
			if value.Null {
				return column + " is not null"
			}
			return fmt.Sprintf("%s < $%d", column, parameter)
		}
		if value.Null {
			return "false"
		}
		return fmt.Sprintf("(%s > $%d or %s is null)", column, parameter, column)
	}
	if before {
		if value.Null {
			return "false"
		}
		return fmt.Sprintf("(%s > $%d or %s is null)", column, parameter, column)
	}
	if value.Null {
		return column + " is not null"
	}
	return fmt.Sprintf("%s < $%d", column, parameter)
}

func orderSuffix(ascending, reverse bool) string {
	if reverse {
		ascending = !ascending
	}
	if ascending {
		return " asc nulls last"
	}
	return " desc nulls first"
}

func collectFetchedRows(rows pgx.Rows, relation grid.Relation, terms []orderTerm) ([]fetchedRow, error) {
	defer rows.Close()
	result := make([]fetchedRow, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		expected := len(relation.Columns) + len(terms)
		if relation.HasXMin {
			expected++
		}
		if len(values) != expected {
			return nil, fmt.Errorf("unexpected table projection: got %d values, want %d", len(values), expected)
		}
		row := grid.Row{Identity: make(map[string]any, len(relation.Identity)), Cells: make([]grid.Cell, len(relation.Columns))}
		for index, column := range relation.Columns {
			row.Cells[index] = formatCell(values[index], column)
			if column.IdentityPart {
				row.Identity[column.Name] = row.Cells[index].Raw
			}
		}
		position := len(relation.Columns)
		if relation.HasXMin {
			switch xmin := values[position].(type) {
			case int64:
				if xmin >= 0 && xmin <= math.MaxUint32 {
					row.XMin = uint32(xmin)
				}
			case uint32:
				row.XMin = xmin
			}
			position++
		}
		cursor := make([]cursorValue, len(terms))
		for index := range terms {
			value := values[position+index]
			if value == nil {
				cursor[index].Null = true
			} else {
				cursor[index].Text = fmt.Sprint(value)
			}
		}
		result = append(result, fetchedRow{row: row, cursor: cursor})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func rowsOnly(rows []fetchedRow) []grid.Row {
	result := make([]grid.Row, len(rows))
	for index := range rows {
		result[index] = rows[index].row
	}
	return result
}

func encodeCursor(cursor pageCursor) (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", core.NewError("encode page cursor", core.ErrorInternal, "Could not prepare the next table page.", true, err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(encoded string) (pageCursor, error) {
	if encoded == "" || len(encoded) > 64*1024 {
		return pageCursor{}, invalidCursorError()
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return pageCursor{}, invalidCursorError()
	}
	var cursor pageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.Version != 1 {
		return pageCursor{}, invalidCursorError()
	}
	return cursor, nil
}

func validateKeysetCursor(cursor pageCursor, relation grid.RelationID, sort grid.Sort, terms []orderTerm) error {
	if cursor.Relation != relation || cursor.Sort != sort || len(cursor.Terms) != len(terms) || len(cursor.Values) != len(terms) {
		return invalidCursorError()
	}
	for index, term := range terms {
		if cursor.Terms[index] != term.Column {
			return invalidCursorError()
		}
	}
	return nil
}

func invalidCursorError() *core.Error {
	return core.NewError("decode page cursor", core.ErrorValidation, "The table page cursor is invalid. Refresh to return to the first page.", true, nil)
}

func relationColumn(relation grid.Relation, name string) (grid.Column, bool) {
	for _, column := range relation.Columns {
		if column.Name == name {
			return column, true
		}
	}
	return grid.Column{}, false
}

func quotedRelation(relation grid.RelationID) string {
	return pgx.Identifier{relation.Schema, relation.Name}.Sanitize()
}

func quotedColumn(column string) string {
	return pgx.Identifier{column}.Sanitize()
}

func gridRelationKind(kind string) grid.RelationKind {
	switch kind {
	case "p":
		return grid.RelationPartitionedTable
	case "v":
		return grid.RelationView
	case "m":
		return grid.RelationMaterializedView
	case "f":
		return grid.RelationForeignTable
	default:
		return grid.RelationTable
	}
}

func formatCell(value any, column grid.Column) grid.Cell {
	if value == nil {
		return grid.Cell{Null: true, Display: "NULL"}
	}
	normalized := normalizeValue(value)
	return grid.Cell{Raw: normalized, Display: formatDisplayValue(normalized, column), Edit: editCellValue(normalized, column)}
}

func editCellValue(value any, column grid.Column) string {
	switch value := value.(type) {
	case string:
		return value
	case []byte:
		if column.TypeOID == 17 || strings.HasPrefix(column.DataType, "bytea") {
			return `\x` + hex.EncodeToString(value)
		}
		return string(value)
	case time.Time:
		if column.TypeOID == 1082 || column.DataType == "date" {
			return value.Format("2006-01-02")
		}
		return value.Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(value)
	}
}

func normalizeValue(value any) any {
	if valuer, ok := value.(driver.Valuer); ok {
		if normalized, err := valuer.Value(); err == nil {
			return normalized
		}
	}
	switch value := value.(type) {
	case time.Time, bool, string, []byte,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return value
	}
}

func formatDisplayValue(value any, column grid.Column) string {
	switch value := value.(type) {
	case string:
		return sanitizeCellText(value)
	case []byte:
		if column.TypeOID == 17 || strings.HasPrefix(column.DataType, "bytea") {
			return `\x` + hex.EncodeToString(value)
		}
		if column.DataType == "json" || column.DataType == "jsonb" {
			var compact bytes.Buffer
			if err := json.Compact(&compact, value); err == nil {
				return sanitizeCellText(compact.String())
			}
		}
		return sanitizeCellText(string(value))
	case time.Time:
		if column.TypeOID == 1082 || column.DataType == "date" {
			return value.Format("2006-01-02")
		}
		return value.Format(time.RFC3339Nano)
	case float32:
		return strconv.FormatFloat(float64(value), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(value, 'g', -1, 64)
	}
	raw := reflect.ValueOf(value)
	if raw.IsValid() && (raw.Kind() == reflect.Array || raw.Kind() == reflect.Slice || raw.Kind() == reflect.Map || raw.Kind() == reflect.Struct) {
		if encoded, err := json.Marshal(value); err == nil {
			return sanitizeCellText(string(encoded))
		}
	}
	return sanitizeCellText(fmt.Sprint(value))
}

func sanitizeCellText(value string) string {
	var builder strings.Builder
	for _, character := range value {
		switch character {
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
		default:
			if unicode.IsControl(character) {
				fmt.Fprintf(&builder, `\u%04x`, character)
			} else {
				builder.WriteRune(character)
			}
		}
	}
	return builder.String()
}

func (session *Session) FetchCurrentRow(ctx context.Context, relationID grid.RelationID, identity map[string]any) (grid.Row, error) {
	relation, err := session.Describe(ctx, relationID)
	if err != nil {
		return grid.Row{}, err
	}
	if len(relation.Identity) == 0 {
		return grid.Row{}, core.NewError("fetch current row", core.ErrorUnsupported, "This relation has no usable row identity.", false, nil)
	}
	conditions := make([]string, len(relation.Identity))
	args := make([]any, len(relation.Identity))
	for index, column := range relation.Identity {
		value, ok := identity[column]
		if !ok {
			return grid.Row{}, core.NewError("fetch current row", core.ErrorValidation, "The row identity is incomplete.", false, nil)
		}
		conditions[index] = fmt.Sprintf("%s is not distinct from $%d", quotedColumn(column), index+1)
		args[index] = value
	}
	query := baseProjection(relation, nil) + " where " + strings.Join(conditions, " and ") + " limit 1"
	rows, err := session.pool.Query(ctx, query, args...)
	if err != nil {
		return grid.Row{}, ClassifyError("fetch current row", err)
	}
	fetched, err := collectFetchedRows(rows, relation, nil)
	if err != nil {
		return grid.Row{}, ClassifyError("read current row", err)
	}
	if len(fetched) == 0 {
		return grid.Row{}, core.NewError("fetch current row", core.ErrorConflict, "The row no longer exists.", false, nil)
	}
	return fetched[0].row, nil
}
