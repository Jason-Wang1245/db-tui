package postgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/launcher"
)

const testTimeout = 10 * time.Second

var _ launcher.Connector = Connector{}

type Connector struct {
	maxConnections int32
	clock          launcher.Clock
}

func NewConnector(clock launcher.Clock, maxConnections int32) Connector {
	if maxConnections < 1 {
		maxConnections = 4
	}
	return Connector{maxConnections: maxConnections, clock: clock}
}

func (connector Connector) Test(ctx context.Context, target launcher.ConnectionTarget, credential launcher.Credential) (launcher.ConnectionInfo, error) {
	testContext, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	started := connector.clock.Now()
	session, info, err := connector.connect(testContext, target, credential)
	if err != nil {
		return launcher.ConnectionInfo{}, err
	}
	session.Close()
	info.Latency = connector.clock.Now().Sub(started)
	return info, nil
}

func (connector Connector) Connect(ctx context.Context, target launcher.ConnectionTarget, credential launcher.Credential) (launcher.Session, launcher.ConnectionInfo, error) {
	started := connector.clock.Now()
	session, info, err := connector.connect(ctx, target, credential)
	if err != nil {
		return nil, launcher.ConnectionInfo{}, err
	}
	info.Latency = connector.clock.Now().Sub(started)
	return session, info, nil
}

func (connector Connector) connect(ctx context.Context, target launcher.ConnectionTarget, credential launcher.Credential) (*Session, launcher.ConnectionInfo, error) {
	config, err := connectionConfig(target, credential, connector.maxConnections)
	if err != nil {
		return nil, launcher.ConnectionInfo{}, ClassifyError("configure connection", err)
	}
	session, err := OpenSession(ctx, config)
	if err != nil {
		return nil, launcher.ConnectionInfo{}, err
	}
	var database string
	var serverVersion string
	if err := session.pool.QueryRow(ctx, "select current_database(), current_setting('server_version')").Scan(&database, &serverVersion); err != nil {
		session.Close()
		return nil, launcher.ConnectionInfo{}, ClassifyError("inspect connection", err)
	}
	return session, launcher.ConnectionInfo{
		ServerVersion: serverVersion,
		Database:      database,
		Server:        net.JoinHostPort(target.Host, strconv.Itoa(int(target.Port))),
	}, nil
}

func connectionConfig(target launcher.ConnectionTarget, credential launcher.Credential, maxConnections int32) (*pgxpool.Config, error) {
	parts := []string{
		"host=" + quoteConnectionValue(target.Host),
		"port=" + strconv.Itoa(int(target.Port)),
		"dbname=" + quoteConnectionValue(target.Database),
		"user=" + quoteConnectionValue(target.User),
		"password=" + quoteConnectionValue(credential.Password),
		"sslmode=" + quoteConnectionValue(target.SSLMode),
		"connect_timeout=10",
		"application_name=db-tui",
	}
	for name, value := range target.AdvancedParameters {
		parts = append(parts, name+"="+quoteConnectionValue(value))
	}
	config, err := pgxpool.ParseConfig(strings.Join(parts, " "))
	if err != nil {
		return nil, err
	}
	config.MaxConns = maxConnections
	return config, nil
}

func quoteConnectionValue(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value)
	return "'" + escaped + "'"
}

func ClassifyError(operation string, err error) *core.Error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return core.NewError(operation, core.ErrorCancellation, operationLabel(operation)+" cancelled.", true, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return core.NewError(operation, core.ErrorTimeout, "PostgreSQL did not respond within 10 seconds. Check the host, port, VPN, and firewall.", true, err)
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		return classifyPostgreSQLError(operation, postgresError, err)
	}
	var hostnameError x509.HostnameError
	var unknownAuthorityError x509.UnknownAuthorityError
	var certificateError x509.CertificateInvalidError
	var tlsHeaderError tls.RecordHeaderError
	if errors.As(err, &hostnameError) || errors.As(err, &unknownAuthorityError) || errors.As(err, &certificateError) || errors.As(err, &tlsHeaderError) {
		return core.NewError(operation, core.ErrorTLS, "TLS verification failed. Check sslmode, the server name, and the configured CA certificate.", false, err)
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return core.NewError(operation, core.ErrorNetwork, "Could not resolve the database host. Check the hostname and DNS connection.", true, err)
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return core.NewError(operation, core.ErrorNetwork, "The server refused the connection. Check that PostgreSQL is running and the port is correct.", true, err)
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return core.NewError(operation, core.ErrorTimeout, "The network connection timed out. Check the host, VPN, and firewall.", true, err)
	}
	return core.NewError(operation, core.ErrorNetwork, "Could not connect to PostgreSQL. Check the connection fields and network access.", true, nil)
}

func classifyPostgreSQLError(operation string, err *pgconn.PgError, cause error) *core.Error {
	details := &core.PostgreSQLDetails{
		SQLState:   sanitizeSQLState(err.Code),
		Detail:     sanitizeServerText(err.Detail),
		Hint:       sanitizeServerText(err.Hint),
		Constraint: sanitizeServerText(err.ConstraintName),
		Position:   err.Position,
		Context:    sanitizeServerText(err.Where),
	}
	category := core.ErrorValidation
	summary := "PostgreSQL rejected " + operationLabel(operation) + "."
	if message := sanitizeServerText(err.Message); message != "" && !isConnectionOperation(operation) {
		summary = "PostgreSQL: " + strings.TrimSuffix(message, ".") + "."
	}
	if details.SQLState != "" && !strings.Contains(summary, "SQLSTATE") {
		summary = strings.TrimSuffix(summary, ".") + fmt.Sprintf(" (SQLSTATE %s).", details.SQLState)
	}
	retryable := false
	switch {
	case strings.HasPrefix(err.Code, "28"):
		category = core.ErrorAuthentication
		summary = "PostgreSQL rejected the username or password. Replace the password and verify the user name."
	case err.Code == "3D000":
		summary = "The database does not exist. Check the database name."
	case err.Code == "42501":
		category = core.ErrorPermission
		if isConnectionOperation(operation) {
			summary = "The user does not have permission to connect to this database."
		}
	case strings.HasPrefix(err.Code, "08"):
		category = core.ErrorNetwork
		summary = "PostgreSQL closed the connection. Check server availability and network access."
		retryable = true
	case strings.HasPrefix(err.Code, "23"):
		category = core.ErrorConstraint
	case err.Code == "40001" || err.Code == "40P01":
		category = core.ErrorConflict
		retryable = true
	}
	classified := core.NewError(operation, category, summary, retryable, cause)
	classified.PostgreSQL = details
	return classified
}

func isConnectionOperation(operation string) bool {
	switch operation {
	case "configure connection", "connect", "inspect connection", "ping":
		return true
	default:
		return false
	}
}

func operationLabel(operation string) string {
	if isConnectionOperation(operation) {
		return "connection attempt"
	}
	return operation
}

func sanitizeSQLState(value string) string {
	if len(value) != 5 {
		return ""
	}
	for _, character := range value {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) {
			return ""
		}
	}
	return value
}

func sanitizeServerText(value string) string {
	result := make([]rune, 0, min(len(value), 2048))
	for _, character := range value {
		if len(result) == 2048 {
			break
		}
		if unicode.IsControl(character) {
			result = append(result, ' ')
		} else {
			result = append(result, character)
		}
	}
	return strings.TrimSpace(string(result))
}
