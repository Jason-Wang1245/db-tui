package postgres

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/grid"
	"github.com/Jason-Wang1245/db-tui/internal/sqltab"
)

const cleanupTimeout = 3 * time.Second

var _ sqltab.SQLExecutor = (*Session)(nil)

func (session *Session) Execute(ctx context.Context, request sqltab.RunRequest) (sqltab.RunResult, error) {
	started := time.Now()
	if strings.TrimSpace(request.Snapshot) == "" {
		return sqltab.RunResult{}, core.NewError(
			"execute SQL", core.ErrorValidation, "The submitted SQL snapshot is empty.", false, nil,
		)
	}
	connection, err := session.pool.Acquire(ctx)
	if err != nil {
		return sqltab.RunResult{}, ClassifyError("acquire SQL execution connection", err)
	}
	defer connection.Release()

	collector, unregister := session.notices.register(connection.Conn().PgConn())
	defer unregister()

	result, executionErr := executeSimpleQuery(ctx, connection, request.Snapshot)
	notices, noticesCutOff := collector.snapshot()
	attachSQLNotices(&result, notices, noticesCutOff)
	result.Duration = time.Since(started)
	warning, cleanupErr := cleanupExecutionConnection(connection)
	result.Warning = joinWarnings(result.Warning, warning)

	if executionErr != nil {
		markLastRowsIncomplete(&result)
		return result, classifyExecutionError(ctx, executionErr)
	}
	if cleanupErr != nil {
		return result, core.NewError(
			"reset SQL execution connection", core.ErrorNetwork,
			"The isolated execution connection could not be reset and was discarded.", true, cleanupErr,
		)
	}
	return result, nil
}

func attachSQLNotices(result *sqltab.RunResult, notices []sqltab.Notice, alreadyCutOff bool) {
	remaining := int64(sqltab.MaxCapturedDataBytes) - result.CapturedBytes
	cutOff := alreadyCutOff
	for _, notice := range notices {
		size := int64(len(notice.Severity) + len(notice.Message) + len(notice.SQLState) + len(notice.Detail) + len(notice.Hint))
		if size > remaining {
			cutOff = true
			break
		}
		result.Notices = append(result.Notices, notice)
		result.CapturedBytes += size
		remaining -= size
	}
	if cutOff {
		result.Warning = joinWarnings(result.Warning, "Notices were truncated at the 64 MiB execution capture limit.")
	}
}

func joinWarnings(first, second string) string {
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + " " + second
}

func executeSimpleQuery(ctx context.Context, connection *pgxpool.Conn, snapshot string) (sqltab.RunResult, error) {
	reader := connection.Conn().PgConn().Exec(ctx, snapshot)
	result := sqltab.RunResult{}
	var captured int64
	outputStarted := time.Now()
	for reader.NextResult() {
		resultReader := reader.ResultReader()
		fields := resultReader.FieldDescriptions()
		if len(fields) == 0 {
			tag, err := resultReader.Close()
			if err != nil {
				if appendServerError(&result, ctx, err, time.Since(outputStarted)) {
					return result, nil
				}
				return result, err
			}
			if tag.String() != "" {
				result.Outputs = append(result.Outputs, sqltab.Output{
					Kind: sqltab.OutputCommand, CommandTag: tag.String(),
					AffectedRows: affectedRows(tag), Duration: time.Since(outputStarted),
				})
			}
			outputStarted = time.Now()
			continue
		}

		output := sqltab.Output{Kind: sqltab.OutputRows, AffectedRows: -1}
		output.Columns = resultColumns(connection, fields)
		for _, column := range output.Columns {
			captured += int64(len(column.Name) + len(column.DataType))
		}
		for resultReader.NextRow() {
			rawValues := resultReader.Values()
			rowBytes := rawValuesSize(rawValues)
			if !canCaptureSQLRow(len(output.Rows), captured, rowBytes) {
				output.Truncated = true
				continue
			}
			row, err := decodeResultRow(connection, fields, rawValues)
			if err != nil {
				resultReader.Close()
				return result, core.NewError(
					"decode SQL result", core.ErrorInternal,
					"A PostgreSQL result value could not be decoded safely.", false, err,
				)
			}
			output.Rows = append(output.Rows, row)
			captured += rowBytes
		}
		tag, err := resultReader.Close()
		output.CommandTag = tag.String()
		output.Duration = time.Since(outputStarted)
		result.Outputs = append(result.Outputs, output)
		if err != nil {
			if appendServerError(&result, ctx, err, 0) {
				result.CapturedBytes = captured
				return result, nil
			}
			result.CapturedBytes = captured
			return result, err
		}
		outputStarted = time.Now()
	}
	err := reader.Close()
	result.CapturedBytes = captured
	if err != nil {
		if appendServerError(&result, ctx, err, time.Since(outputStarted)) {
			return result, nil
		}
		return result, err
	}
	return result, nil
}

func canCaptureSQLRow(capturedRows int, capturedBytes, rowBytes int64) bool {
	return capturedRows < sqltab.MaxRowsPerResult && rowBytes >= 0 &&
		capturedBytes <= sqltab.MaxCapturedDataBytes-rowBytes
}

func resultColumns(connection *pgxpool.Conn, fields []pgconn.FieldDescription) []sqltab.Column {
	columns := make([]sqltab.Column, len(fields))
	typeMap := connection.Conn().TypeMap()
	for index, field := range fields {
		dataType := fmt.Sprintf("oid %d", field.DataTypeOID)
		if registered, ok := typeMap.TypeForOID(field.DataTypeOID); ok {
			dataType = registered.Name
		}
		columns[index] = sqltab.Column{Name: field.Name, DataType: dataType, TypeOID: field.DataTypeOID}
	}
	return columns
}

func decodeResultRow(connection *pgxpool.Conn, fields []pgconn.FieldDescription, values [][]byte) ([]sqltab.Cell, error) {
	row := make([]sqltab.Cell, len(fields))
	typeMap := connection.Conn().TypeMap()
	for index, field := range fields {
		if values[index] == nil {
			row[index] = sqltab.Cell{Null: true, Display: "NULL", Full: "NULL"}
			continue
		}
		var raw any
		if err := typeMap.Scan(field.DataTypeOID, field.Format, values[index], &raw); err != nil {
			raw = string(values[index])
		}
		dataType := ""
		if registered, ok := typeMap.TypeForOID(field.DataTypeOID); ok {
			dataType = registered.Name
		}
		column := grid.Column{TypeOID: field.DataTypeOID, DataType: dataType}
		formatted := formatCell(raw, column)
		row[index] = sqltab.Cell{
			Raw: formatted.Raw, Display: formatted.Display, Full: formatFullSQLValue(raw, column), Null: formatted.Null,
		}
	}
	return row, nil
}

func formatFullSQLValue(value any, column grid.Column) string {
	if value == nil {
		return "NULL"
	}
	normalized := normalizeValue(value)
	switch value := normalized.(type) {
	case string:
		return sanitizeMultilineCellText(value)
	case []byte:
		if column.TypeOID == 17 || strings.HasPrefix(column.DataType, "bytea") {
			return `\x` + hex.EncodeToString(value)
		}
		return sanitizeMultilineCellText(string(value))
	default:
		return formatDisplayValue(normalized, column)
	}
}

func sanitizeMultilineCellText(value string) string {
	var builder strings.Builder
	for _, character := range value {
		switch character {
		case '\n':
			builder.WriteRune(character)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteRune(character)
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

func rawValuesSize(values [][]byte) int64 {
	var size int64
	for _, value := range values {
		size += int64(len(value))
	}
	return size
}

func affectedRows(tag pgconn.CommandTag) int64 {
	fields := strings.Fields(tag.String())
	if len(fields) == 0 {
		return -1
	}
	command := strings.ToUpper(fields[0])
	switch command {
	case "INSERT", "UPDATE", "DELETE", "MERGE", "COPY", "MOVE", "FETCH":
		return tag.RowsAffected()
	default:
		return -1
	}
}

func appendServerError(result *sqltab.RunResult, ctx context.Context, err error, duration time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return false
	}
	classified := ClassifyError("execute SQL", err)
	result.Outputs = append(result.Outputs, sqltab.Output{
		Kind: sqltab.OutputError, Error: classified, Duration: duration,
	})
	return true
}

func classifyExecutionError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ClassifyError("execute SQL", ctx.Err())
	}
	var structured *core.Error
	if errors.As(err, &structured) {
		return structured
	}
	return ClassifyError("execute SQL", err)
}

func cleanupExecutionConnection(connection *pgxpool.Conn) (string, error) {
	if connection.Conn().IsClosed() {
		return "The execution connection was discarded.", nil
	}
	cleanupContext, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	warning := ""
	if connection.Conn().PgConn().TxStatus() != 'I' {
		warning = "The batch left a transaction open; db-tui rolled it back."
		if _, err := connection.Exec(cleanupContext, "rollback"); err != nil {
			connection.Conn().Close(cleanupContext)
			return warning + " The connection was discarded.", err
		}
	}
	if _, err := connection.Exec(cleanupContext, "discard all"); err != nil {
		connection.Conn().Close(cleanupContext)
		return warning + " The connection was discarded.", err
	}
	return warning, nil
}

func markLastRowsIncomplete(result *sqltab.RunResult) {
	for index := len(result.Outputs) - 1; index >= 0; index-- {
		if result.Outputs[index].Kind == sqltab.OutputRows {
			result.Outputs[index].Incomplete = true
			return
		}
	}
}
