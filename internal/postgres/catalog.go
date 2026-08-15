package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Jason-Wang1245/db-tui/internal/workspace"
)

var _ workspace.CatalogReader = (*Session)(nil)

const schemasQuery = `
select n.nspname
from pg_catalog.pg_namespace n
where n.nspname <> 'information_schema'
  and n.nspname not like 'pg_%'
  and pg_catalog.has_schema_privilege(n.oid, 'USAGE')
order by n.nspname`

const relationsQuery = `
select
  n.nspname,
  c.relname,
  c.relkind::text,
  pg_catalog.has_table_privilege(c.oid, 'SELECT')
from pg_catalog.pg_class c
join pg_catalog.pg_namespace n on n.oid = c.relnamespace
where n.nspname = $1
  and c.relkind in ('r', 'p', 'v', 'm', 'f')
  and pg_catalog.has_schema_privilege(n.oid, 'USAGE')
order by c.relname`

func (session *Session) Schemas(ctx context.Context) ([]workspace.Schema, error) {
	rows, err := session.pool.Query(ctx, schemasQuery)
	if err != nil {
		return nil, ClassifyError("load schemas", err)
	}
	schemas, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (workspace.Schema, error) {
		var schema workspace.Schema
		err := row.Scan(&schema.Name)
		return schema, err
	})
	if err != nil {
		return nil, ClassifyError("load schemas", err)
	}
	return schemas, nil
}

func (session *Session) Relations(ctx context.Context, schema string) ([]workspace.Relation, error) {
	rows, err := session.pool.Query(ctx, relationsQuery, schema)
	if err != nil {
		return nil, ClassifyError("load relations", err)
	}
	relations, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (workspace.Relation, error) {
		var relation workspace.Relation
		var kind string
		err := row.Scan(&relation.Schema, &relation.Name, &kind, &relation.CanSelect)
		relation.Kind = relationKind(kind)
		return relation, err
	})
	if err != nil {
		return nil, ClassifyError("load relations", err)
	}
	return relations, nil
}

func relationKind(kind string) workspace.RelationKind {
	switch kind {
	case "p":
		return workspace.RelationPartitionedTable
	case "v":
		return workspace.RelationView
	case "m":
		return workspace.RelationMaterializedView
	case "f":
		return workspace.RelationForeignTable
	default:
		return workspace.RelationTable
	}
}
