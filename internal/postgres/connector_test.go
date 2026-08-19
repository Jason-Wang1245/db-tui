package postgres

import (
	"context"
	"crypto/x509"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/launcher"
)

func TestConnectionConfigKeepsFieldsAndPasswordSeparate(t *testing.T) {
	target := launcher.ConnectionTarget{
		Host: "db.example.com", Port: 6543, Database: "odd db", User: "alice",
		SSLMode: "require", AdvancedParameters: map[string]string{"search_path": "private,public"},
	}
	config, err := connectionConfig(target, launcher.Credential{Password: "p'ass\\word"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Host != target.Host || config.ConnConfig.Port != target.Port || config.ConnConfig.Database != target.Database || config.ConnConfig.User != target.User {
		t.Fatalf("config = %#v", config.ConnConfig.Config)
	}
	if config.ConnConfig.Password != "p'ass\\word" || config.ConnConfig.RuntimeParams["search_path"] != "private,public" {
		t.Fatalf("password or parameters were parsed incorrectly")
	}
	if config.MaxConns != 3 || config.ConnConfig.ConnectTimeout.Seconds() != 10 {
		t.Fatalf("pool limits = %d, timeout = %s", config.MaxConns, config.ConnConfig.ConnectTimeout)
	}
}

func TestClassifyErrorProvidesActionableSafeCategories(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		category core.ErrorCategory
	}{
		{"cancelled", context.Canceled, core.ErrorCancellation},
		{"timeout", context.DeadlineExceeded, core.ErrorTimeout},
		{"authentication", &pgconn.PgError{Code: "28P01", Message: "password authentication failed"}, core.ErrorAuthentication},
		{"database", &pgconn.PgError{Code: "3D000", Message: "database does not exist"}, core.ErrorValidation},
		{"tls", x509.HostnameError{Certificate: &x509.Certificate{}, Host: "wrong.example.com"}, core.ErrorTLS},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			classified := ClassifyError("connect", test.err)
			if classified.Category != test.category || classified.Summary == "" {
				t.Fatalf("classified = %#v", classified)
			}
		})
	}
}

func TestClassifiedErrorDoesNotExposeWrappedCredential(t *testing.T) {
	secret := "never-show-this"
	err := ClassifyError("connect", errors.New("dial failed with password "+secret))
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Summary, secret) {
		t.Fatalf("classified error exposed credential: %v", err)
	}
	if wrapped := errors.Unwrap(err); wrapped != nil && strings.Contains(wrapped.Error(), secret) {
		t.Fatalf("wrapped error exposed credential: %v", wrapped)
	}
}

func TestClassifyErrorSanitizesPostgreSQLDetails(t *testing.T) {
	classified := ClassifyError("connect", &pgconn.PgError{
		Code: "22P02", Detail: "bad\x1b[31mvalue\nnext", Hint: strings.Repeat("x", 3000),
	})
	if strings.ContainsRune(classified.PostgreSQL.Detail, '\x1b') || strings.ContainsRune(classified.PostgreSQL.Detail, '\n') {
		t.Fatalf("detail contains terminal controls: %q", classified.PostgreSQL.Detail)
	}
	if len([]rune(classified.PostgreSQL.Hint)) != 2048 {
		t.Fatalf("hint length = %d", len([]rune(classified.PostgreSQL.Hint)))
	}
}

func TestClassifyErrorShowsPostgreSQLBrowseMessage(t *testing.T) {
	classified := ClassifyError("fetch table page", &pgconn.PgError{
		Code: "22P02", Message: "invalid input syntax for type integer",
	})
	if !strings.Contains(classified.Summary, "invalid input syntax for type integer") ||
		!strings.Contains(classified.Summary, "SQLSTATE 22P02") {
		t.Fatalf("browse summary = %q", classified.Summary)
	}

	permission := ClassifyError("fetch table page", &pgconn.PgError{
		Code: "42501", Message: "permission denied for table users",
	})
	if permission.Category != core.ErrorPermission || !strings.Contains(permission.Summary, "permission denied for table users") {
		t.Fatalf("permission browse error = %#v", permission)
	}
}

func TestClassifyErrorCategorizesConstraintAndTransactionConflicts(t *testing.T) {
	constraint := ClassifyError("apply staged changes", &pgconn.PgError{Code: "23505", Message: "duplicate key"})
	if constraint.Category != core.ErrorConstraint || constraint.PostgreSQL == nil {
		t.Fatalf("constraint classification = %#v", constraint)
	}
	conflict := ClassifyError("apply staged changes", &pgconn.PgError{Code: "40001", Message: "serialization failure"})
	if conflict.Category != core.ErrorConflict || !conflict.Retryable {
		t.Fatalf("transaction conflict classification = %#v", conflict)
	}
}
