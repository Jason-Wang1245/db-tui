//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

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
}
