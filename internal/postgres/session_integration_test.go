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
