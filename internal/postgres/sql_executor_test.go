package postgres

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Jason-Wang1245/db-tui/internal/sqltab"
)

func TestSQLCaptureLimitsAreStrict(t *testing.T) {
	if !canCaptureSQLRow(sqltab.MaxRowsPerResult-1, sqltab.MaxCapturedDataBytes-1, 1) {
		t.Fatal("last permitted row byte was rejected")
	}
	if canCaptureSQLRow(sqltab.MaxRowsPerResult, 0, 1) {
		t.Fatal("row count beyond the result cap was accepted")
	}
	if canCaptureSQLRow(0, sqltab.MaxCapturedDataBytes, 1) {
		t.Fatal("data beyond the execution cap was accepted")
	}
	if canCaptureSQLRow(0, 0, sqltab.MaxCapturedDataBytes+1) {
		t.Fatal("oversized first row was accepted")
	}
}

func TestSQLCommandAffectedRowsAndFullValueSafety(t *testing.T) {
	if got := affectedRows(pgconn.NewCommandTag("UPDATE 7")); got != 7 {
		t.Fatalf("UPDATE affected rows = %d", got)
	}
	if got := affectedRows(pgconn.NewCommandTag("CREATE TABLE")); got != -1 {
		t.Fatalf("DDL affected rows = %d", got)
	}
	full := sanitizeMultilineCellText("line one\nline two\x1b")
	if !strings.Contains(full, "\n") || strings.ContainsRune(full, '\x1b') || !strings.Contains(full, `\u001b`) {
		t.Fatalf("safe full value = %q", full)
	}
}

func TestSQLNoticesShareTheExecutionCaptureLimit(t *testing.T) {
	result := sqltab.RunResult{CapturedBytes: sqltab.MaxCapturedDataBytes - 2}
	attachSQLNotices(&result, []sqltab.Notice{{Message: "four"}}, false)
	if len(result.Notices) != 0 || !strings.Contains(result.Warning, "64 MiB") || result.CapturedBytes != sqltab.MaxCapturedDataBytes-2 {
		t.Fatalf("limited notices result = %#v", result)
	}
}
