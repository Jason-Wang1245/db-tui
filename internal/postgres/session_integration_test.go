//go:build integration

package postgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/grid"
	"github.com/Jason-Wang1245/db-tui/internal/launcher"
	"github.com/Jason-Wang1245/db-tui/internal/sqltab"
)

type integrationClock struct{}

func (integrationClock) Now() time.Time { return time.Now() }

func TestSessionConnectsToSupportedPostgreSQL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	major := os.Getenv("TEST_POSTGRES_VERSION")
	if major == "" {
		major = "18"
	}
	container, err := tcpostgres.Run(
		ctx,
		"postgres:"+major+"-alpine",
		tcpostgres.WithDatabase("db_tui_test"),
		tcpostgres.WithUsername("db_tui"),
		tcpostgres.WithPassword("integration-only"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL %s: %v", major, err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("terminate PostgreSQL: %v", err)
		}
	})

	connectionString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	config, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		t.Fatalf("parse connection config: %v", err)
	}
	config.MaxConns = 4

	session, err := OpenSession(ctx, config)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	t.Cleanup(session.Close)
	if err := session.Ping(ctx); err != nil {
		t.Fatalf("ping session: %v", err)
	}
	if _, err := session.pool.Exec(ctx, `
		create schema audit;
		create table public.users (id bigint primary key, email text not null);
		create view public.active_users as select id, email from public.users;
		create table audit.events (id bigint primary key);
	`); err != nil {
		t.Fatalf("create catalog fixtures: %v", err)
	}
	schemas, err := session.Schemas(ctx)
	if err != nil {
		t.Fatalf("load schemas: %v", err)
	}
	if len(schemas) != 2 || schemas[0].Name != "audit" || schemas[1].Name != "public" {
		t.Fatalf("schemas = %#v", schemas)
	}
	relations, err := session.Relations(ctx, "public")
	if err != nil {
		t.Fatalf("load public relations: %v", err)
	}
	if len(relations) != 2 || relations[0].Name != "active_users" || relations[1].Name != "users" {
		t.Fatalf("relations = %#v", relations)
	}
	if !relations[0].CanSelect || !relations[1].CanSelect {
		t.Fatalf("owner privileges = %#v", relations)
	}
	if _, err := session.pool.Exec(ctx, `
		alter table public.users
		  add column score integer,
		  add column active boolean not null default true,
		  add column created_at timestamptz not null default now(),
		  add column payload jsonb,
		  add column raw bytea;
		insert into public.users (id, email, score, active, created_at, payload, raw)
		select
		  value,
		  'user-' || lpad(value::text, 3, '0') || '@example.com',
		  case when value % 10 = 0 then null else value % 7 end,
		  value % 2 = 0,
		  timestamptz '2026-01-01 00:00:00+00' + value * interval '1 minute',
		  jsonb_build_object('id', value, 'label', E'line one\nline two'),
		  decode(lpad(to_hex(value), 4, '0'), 'hex')
		from generate_series(1, 235) value;
		create table public.keyless_records (note text, amount integer);
		insert into public.keyless_records
		select 'note-' || value, value from generate_series(1, 125) value;
		create table public."odd""name" (id integer primary key, value text);
		insert into public."odd""name" values (1, E'hello\nworld');
		create table public.crud_records (
		  id bigint primary key,
		  identity_value bigint generated always as identity,
		  required_text text not null,
		  optional_text text,
		  count_value integer not null default 7,
		  payload jsonb,
		  generated_text text generated always as (required_text || '!') stored
		);
		create table public.slow_crud (id bigint primary key);
		create function public.slow_crud_insert() returns trigger language plpgsql as $$
		begin
		  perform pg_sleep(10);
		  return new;
		end $$;
		create trigger slow_crud_insert before insert on public.slow_crud
		for each row execute function public.slow_crud_insert();
	`); err != nil {
		t.Fatalf("create browsing fixtures: %v", err)
	}

	users, err := session.Describe(ctx, grid.RelationID{Schema: "public", Name: "users"})
	if err != nil {
		t.Fatalf("describe users: %v", err)
	}
	if !users.IdentityPrimary || len(users.Identity) != 1 || users.Identity[0] != "id" || users.BestEffort || len(users.Columns) != 7 {
		t.Fatalf("users metadata = %#v", users)
	}
	first, err := session.FetchPage(ctx, users, grid.PageRequest{
		Relation: users.ID, Direction: grid.PageFirst, Limit: 100,
	})
	if err != nil {
		t.Fatalf("first users page: %v", err)
	}
	if len(first.Rows) != 100 || first.NextCursor == "" || first.PrevCursor != "" || first.Rows[0].Identity["id"] != int64(1) || first.Rows[0].XMin == 0 {
		t.Fatalf("first users page = %#v", first)
	}
	second, err := session.FetchPage(ctx, users, grid.PageRequest{
		Relation: users.ID, Cursor: first.NextCursor, Direction: grid.PageNext, Limit: 100,
	})
	if err != nil {
		t.Fatalf("second users page: %v", err)
	}
	if len(second.Rows) != 100 || second.NextCursor == "" || second.PrevCursor == "" || second.Rows[0].Identity["id"] != int64(101) {
		t.Fatalf("second users page = %#v", second)
	}
	back, err := session.FetchPage(ctx, users, grid.PageRequest{
		Relation: users.ID, Cursor: second.PrevCursor, Direction: grid.PagePrevious, Limit: 100,
	})
	if err != nil {
		t.Fatalf("previous users page: %v", err)
	}
	if len(back.Rows) != 100 || back.Rows[0].Identity["id"] != int64(1) || back.PrevCursor != "" {
		t.Fatalf("previous users page = %#v", back)
	}
	sorted, err := session.FetchPage(ctx, users, grid.PageRequest{
		Relation: users.ID, Direction: grid.PageFirst, Sort: grid.Sort{Column: "score", Ascending: false}, Limit: 25,
	})
	if err != nil {
		t.Fatalf("sorted users page: %v", err)
	}
	if len(sorted.Rows) != 25 || !sorted.Rows[0].Cells[2].Null || sorted.NextCursor == "" {
		t.Fatalf("nullable sorted page = %#v", sorted)
	}

	keyless, err := session.Describe(ctx, grid.RelationID{Schema: "public", Name: "keyless_records"})
	if err != nil {
		t.Fatalf("describe keyless relation: %v", err)
	}
	if !keyless.BestEffort || len(keyless.Identity) != 0 {
		t.Fatalf("keyless metadata = %#v", keyless)
	}
	keylessFirst, err := session.FetchPage(ctx, keyless, grid.PageRequest{Relation: keyless.ID, Direction: grid.PageFirst, Limit: 100})
	if err != nil {
		t.Fatalf("first keyless page: %v", err)
	}
	keylessSecond, err := session.FetchPage(ctx, keyless, grid.PageRequest{
		Relation: keyless.ID, Cursor: keylessFirst.NextCursor, Direction: grid.PageNext, Limit: 100,
	})
	if err != nil {
		t.Fatalf("second keyless page: %v", err)
	}
	if !keylessFirst.BestEffort || len(keylessFirst.Rows) != 100 || len(keylessSecond.Rows) != 25 || keylessSecond.PrevCursor == "" {
		t.Fatalf("keyless pages = first %#v second %#v", keylessFirst, keylessSecond)
	}

	odd, err := session.Describe(ctx, grid.RelationID{Schema: "public", Name: `odd"name`})
	if err != nil {
		t.Fatalf("describe quoted relation: %v", err)
	}
	oddPage, err := session.FetchPage(ctx, odd, grid.PageRequest{Relation: odd.ID, Direction: grid.PageFirst, Limit: 100})
	if err != nil {
		t.Fatalf("fetch quoted relation: %v", err)
	}
	if len(oddPage.Rows) != 1 || oddPage.Rows[0].Cells[1].Display != `hello\nworld` {
		t.Fatalf("quoted relation page = %#v", oddPage)
	}

	view, err := session.Describe(ctx, grid.RelationID{Schema: "public", Name: "active_users"})
	if err != nil {
		t.Fatalf("describe view: %v", err)
	}
	if view.HasXMin || !view.BestEffort || view.Kind != grid.RelationView {
		t.Fatalf("view metadata = %#v", view)
	}

	crud, err := session.Describe(ctx, grid.RelationID{Schema: "public", Name: "crud_records"})
	if err != nil {
		t.Fatalf("describe CRUD fixture: %v", err)
	}
	if !crud.CanInsert || !crud.CanUpdate || !crud.CanDelete || len(crud.Identity) != 1 || len(crud.Columns) != 7 ||
		!crud.Columns[0].CanUpdate || !crud.Columns[1].Identity || crud.Columns[1].CanInsert || !crud.Columns[6].Generated || crud.Columns[6].CanUpdate {
		t.Fatalf("CRUD metadata = %#v", crud)
	}
	inserted, err := session.Apply(ctx, grid.ApplyRequest{Relation: crud, Mutations: []grid.Mutation{{
		Kind: grid.MutationInsert, DraftID: 1,
		Values: map[string]grid.StagedValue{
			"id":            {Kind: grid.ValueText, Text: "1"},
			"required_text": {Kind: grid.ValueText, Text: ""},
			"optional_text": {Kind: grid.ValueNull},
			"payload":       {Kind: grid.ValueText, Text: `{"ok":true}`},
		},
	}}})
	if err != nil || inserted.Inserted != 1 {
		t.Fatalf("insert staged CRUD row: result=%#v err=%v", inserted, err)
	}
	crudPage, err := session.FetchPage(ctx, crud, grid.PageRequest{Relation: crud.ID, Direction: grid.PageFirst, Limit: 100})
	if err != nil || len(crudPage.Rows) != 1 {
		t.Fatalf("fetch inserted CRUD row: page=%#v err=%v", crudPage, err)
	}
	crudRow := crudPage.Rows[0]
	if crudRow.Cells[2].Null || crudRow.Cells[2].Edit != "" || !crudRow.Cells[3].Null || crudRow.Cells[4].Display != "7" || crudRow.Cells[6].Display != "!" {
		t.Fatalf("insert NULL/empty/default/generated values = %#v", crudRow.Cells)
	}
	updated, err := session.Apply(ctx, grid.ApplyRequest{Relation: crud, Mutations: []grid.Mutation{{
		Kind: grid.MutationUpdate, Original: crudRow,
		Values: map[string]grid.StagedValue{
			"id":            {Kind: grid.ValueText, Text: "5"},
			"required_text": {Kind: grid.ValueText, Text: "updated"},
			"count_value":   {Kind: grid.ValueText, Text: "9"},
		},
	}}})
	if err != nil || updated.Updated != 1 {
		t.Fatalf("update staged CRUD row: result=%#v err=%v", updated, err)
	}
	current, err := session.FetchCurrentRow(ctx, crud.ID, map[string]any{"id": int64(5)})
	if err != nil || current.Cells[2].Display != "updated" || current.Cells[4].Display != "9" || current.Cells[6].Display != "updated!" {
		t.Fatalf("updated/default-generated CRUD row=%#v err=%v", current, err)
	}
	if _, err := session.pool.Exec(ctx, `update public.crud_records set optional_text = 'concurrent' where id = 5`); err != nil {
		t.Fatalf("create optimistic conflict: %v", err)
	}
	_, err = session.Apply(ctx, grid.ApplyRequest{Relation: crud, Mutations: []grid.Mutation{{
		Kind: grid.MutationUpdate, Original: current,
		Values: map[string]grid.StagedValue{"count_value": {Kind: grid.ValueText, Text: "10"}},
	}}})
	var conflict *grid.MutationError
	var conflictCore *core.Error
	if !errors.As(err, &conflict) || !errors.As(err, &conflictCore) || conflictCore.Category != core.ErrorConflict ||
		conflict.Current == nil || conflict.Current.Cells[3].Display != "concurrent" {
		t.Fatalf("optimistic conflict = %#v core=%#v", err, conflictCore)
	}
	current, err = session.FetchCurrentRow(ctx, crud.ID, map[string]any{"id": int64(5)})
	if err != nil {
		t.Fatalf("reload current CRUD row: %v", err)
	}
	_, err = session.Apply(ctx, grid.ApplyRequest{Relation: crud, Mutations: []grid.Mutation{
		{Kind: grid.MutationUpdate, Original: current, Values: map[string]grid.StagedValue{"count_value": {Kind: grid.ValueText, Text: "11"}}},
		{Kind: grid.MutationInsert, DraftID: 2, Values: map[string]grid.StagedValue{"payload": {Kind: grid.ValueText, Text: "not-json"}}},
	}})
	var castFailure *grid.MutationError
	var castCore *core.Error
	if !errors.As(err, &castFailure) || castFailure.Mutation != 1 || castFailure.Column != "payload" ||
		!errors.As(err, &castCore) || castCore.PostgreSQL == nil || castCore.PostgreSQL.SQLState != "22P02" {
		t.Fatalf("raw PostgreSQL cast failure = %#v core=%#v", err, castCore)
	}
	current, err = session.FetchCurrentRow(ctx, crud.ID, map[string]any{"id": int64(5)})
	if err != nil || current.Cells[4].Display != "9" {
		t.Fatalf("failed batch was not fully rolled back: row=%#v err=%v", current, err)
	}
	deleted, err := session.Apply(ctx, grid.ApplyRequest{Relation: crud, Mutations: []grid.Mutation{{
		Kind: grid.MutationDelete, Original: current,
	}}})
	if err != nil || deleted.Deleted != 1 {
		t.Fatalf("delete staged CRUD row: result=%#v err=%v", deleted, err)
	}
	slowCRUD, err := session.Describe(ctx, grid.RelationID{Schema: "public", Name: "slow_crud"})
	if err != nil {
		t.Fatalf("describe cancellation fixture: %v", err)
	}
	applyContext, cancelApply := context.WithCancel(ctx)
	applyCancelled := make(chan error, 1)
	go func() {
		_, applyErr := session.Apply(applyContext, grid.ApplyRequest{Relation: slowCRUD, Mutations: []grid.Mutation{{
			Kind:   grid.MutationInsert,
			Values: map[string]grid.StagedValue{"id": {Kind: grid.ValueText, Text: "1"}},
		}}})
		applyCancelled <- applyErr
	}()
	time.Sleep(100 * time.Millisecond)
	cancelApply()
	var cancelledMutation *grid.MutationError
	var cancelledApplyCore *core.Error
	if err := <-applyCancelled; !errors.As(err, &cancelledMutation) || !errors.As(err, &cancelledApplyCore) || cancelledApplyCore.Category != core.ErrorCancellation {
		t.Fatalf("cancelled apply error = %#v", err)
	}
	slowPage, err := session.FetchPage(ctx, slowCRUD, grid.PageRequest{Relation: slowCRUD.ID, Direction: grid.PageFirst, Limit: 100})
	if err != nil || len(slowPage.Rows) != 0 {
		t.Fatalf("cancelled apply was not rolled back: page=%#v err=%v", slowPage, err)
	}

	batch, err := session.Execute(ctx, sqltab.RunRequest{Snapshot: `
		do $$ begin raise notice 'ordered notice'; end $$;
		select 1::integer as n, null::text as missing, ''::text as empty;
		update public.users set active = false where id = 1;
		select nonexistent_column;
		select 99;
	`})
	if err != nil {
		t.Fatalf("execute ordered SQL batch: %v", err)
	}
	if len(batch.Outputs) != 4 || batch.Outputs[0].Kind != sqltab.OutputCommand ||
		batch.Outputs[1].Kind != sqltab.OutputRows || batch.Outputs[2].CommandTag != "UPDATE 1" ||
		batch.Outputs[3].Kind != sqltab.OutputError {
		t.Fatalf("ordered SQL outputs = %#v", batch.Outputs)
	}
	row := batch.Outputs[1].Rows[0]
	if row[0].Display != "1" || !row[1].Null || row[2].Null || row[2].Full != "" {
		t.Fatalf("SQL row NULL/empty values = %#v", row)
	}
	if len(batch.Notices) != 1 || batch.Notices[0].Message != "ordered notice" {
		t.Fatalf("SQL notices = %#v", batch.Notices)
	}
	if batch.Outputs[3].Error == nil || batch.Outputs[3].Error.PostgreSQL == nil ||
		batch.Outputs[3].Error.PostgreSQL.SQLState != "42703" || batch.Outputs[3].Error.PostgreSQL.Position == 0 {
		t.Fatalf("SQL error output = %#v", batch.Outputs[3])
	}

	openTransaction, err := session.Execute(ctx, sqltab.RunRequest{Snapshot: `
		begin;
		create temp table run_local (id integer);
		insert into run_local values (1);
	`})
	if err != nil || !strings.Contains(openTransaction.Warning, "rolled it back") {
		t.Fatalf("open transaction cleanup result=%#v err=%v", openTransaction, err)
	}
	isolation, err := session.Execute(ctx, sqltab.RunRequest{Snapshot: `select to_regclass('pg_temp.run_local')`})
	if err != nil || len(isolation.Outputs) != 1 || !isolation.Outputs[0].Rows[0][0].Null {
		t.Fatalf("isolated follow-up result=%#v err=%v", isolation, err)
	}

	bounded, err := session.Execute(ctx, sqltab.RunRequest{Snapshot: `select value from generate_series(1, 10005) value`})
	if err != nil || len(bounded.Outputs) != 1 || len(bounded.Outputs[0].Rows) != sqltab.MaxRowsPerResult || !bounded.Outputs[0].Truncated {
		t.Fatalf("bounded SQL result rows=%d output=%#v err=%v", len(bounded.Outputs[0].Rows), bounded.Outputs[0], err)
	}

	cancelContext, cancelRun := context.WithCancel(ctx)
	cancelled := make(chan error, 1)
	go func() {
		_, runErr := session.Execute(cancelContext, sqltab.RunRequest{Snapshot: `select pg_sleep(10)`})
		cancelled <- runErr
	}()
	time.Sleep(100 * time.Millisecond)
	cancelRun()
	var cancelError *core.Error
	if err := <-cancelled; !errors.As(err, &cancelError) || cancelError.Category != core.ErrorCancellation {
		t.Fatalf("cancelled SQL error = %#v", err)
	}
	if err := session.Ping(ctx); err != nil {
		t.Fatalf("pool did not recover after SQL cancellation: %v", err)
	}
}

func TestConnectorReportsServerAndClassifiesAuthentication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	major := os.Getenv("TEST_POSTGRES_VERSION")
	if major == "" {
		major = "18"
	}
	container, err := tcpostgres.Run(
		ctx,
		"postgres:"+major+"-alpine",
		tcpostgres.WithDatabase("db_tui_test"),
		tcpostgres.WithUsername("db_tui"),
		tcpostgres.WithPassword("integration-only"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL %s: %v", major, err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("terminate PostgreSQL: %v", err)
		}
	})

	connectionString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(connectionString)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.ParseUint(parsed.Port(), 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	target := launcher.ConnectionTarget{
		Host: parsed.Hostname(), Port: uint16(port), Database: "db_tui_test", User: "db_tui", SSLMode: "disable",
	}
	connector := NewConnector(integrationClock{}, 2)
	info, err := connector.Test(ctx, target, launcher.Credential{Password: "integration-only"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Database != "db_tui_test" || info.ServerVersion == "" || info.Latency <= 0 {
		t.Fatalf("connection info = %#v", info)
	}

	_, err = connector.Test(ctx, target, launcher.Credential{Password: "wrong-password"})
	var classified *core.Error
	if !errors.As(err, &classified) || classified.Category != core.ErrorAuthentication {
		t.Fatalf("authentication error = %#v", err)
	}
	if strings.Contains(err.Error(), "wrong-password") {
		t.Fatalf("authentication error leaked password: %v", err)
	}
}
