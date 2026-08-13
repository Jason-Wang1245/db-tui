package core

import "fmt"

type ErrorCategory string

const (
	ErrorValidation     ErrorCategory = "validation"
	ErrorAuthentication ErrorCategory = "authentication"
	ErrorNetwork        ErrorCategory = "network"
	ErrorTLS            ErrorCategory = "tls"
	ErrorTimeout        ErrorCategory = "timeout"
	ErrorCancellation   ErrorCategory = "cancellation"
	ErrorPermission     ErrorCategory = "permission"
	ErrorConflict       ErrorCategory = "conflict"
	ErrorConstraint     ErrorCategory = "constraint"
	ErrorUnsupported    ErrorCategory = "unsupported"
	ErrorPersistence    ErrorCategory = "persistence"
	ErrorKeychain       ErrorCategory = "keychain"
	ErrorInternal       ErrorCategory = "internal"
)

type PostgreSQLDetails struct {
	SQLState   string
	Detail     string
	Hint       string
	Constraint string
	Position   int32
	Context    string
}

type Error struct {
	Operation  string
	Category   ErrorCategory
	Summary    string
	Retryable  bool
	PostgreSQL *PostgreSQLDetails
	cause      error
}

func NewError(operation string, category ErrorCategory, summary string, retryable bool, cause error) *Error {
	return &Error{
		Operation: operation,
		Category:  category,
		Summary:   summary,
		Retryable: retryable,
		cause:     cause,
	}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Operation == "" {
		return e.Summary
	}
	return fmt.Sprintf("%s: %s", e.Operation, e.Summary)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}
