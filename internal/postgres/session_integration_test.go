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
