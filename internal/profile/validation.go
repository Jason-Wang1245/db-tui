package profile

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type FieldError struct {
	Field   string
	Message string
}

type ValidationErrors []FieldError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e))
	for _, field := range e {
		parts = append(parts, field.Field+": "+field.Message)
	}
	return strings.Join(parts, "; ")
}

func (e ValidationErrors) For(field string) string {
	for _, candidate := range e {
		if candidate.Field == field {
			return candidate.Message
		}
	}
	return ""
}

var validSSLModes = map[string]bool{
	"disable":     true,
	"allow":       true,
	"prefer":      true,
	"require":     true,
	"verify-ca":   true,
	"verify-full": true,
}

var reservedParameters = map[string]bool{
	"dbname":          true,
	"database":        true,
	"connect_timeout": true,
	"host":            true,
	"password":        true,
	"passfile":        true,
	"port":            true,
	"sslmode":         true,
	"sslpassword":     true,
	"user":            true,
}

func ValidateDraft(draft Draft, existing Document) (Profile, error) {
	var validation ValidationErrors
	name := strings.TrimSpace(draft.Name)
	host := strings.TrimSpace(draft.Host)
	database := strings.TrimSpace(draft.Database)
	user := strings.TrimSpace(draft.User)
	sslMode := strings.ToLower(strings.TrimSpace(draft.SSLMode))

	if name == "" {
		validation = append(validation, FieldError{Field: "name", Message: "is required"})
	}
	if containsControl(name) {
		validation = append(validation, FieldError{Field: "name", Message: "must not contain control characters"})
	}
	if utf8.RuneCountInString(name) > 256 {
		validation = append(validation, FieldError{Field: "name", Message: "must be 256 characters or fewer"})
	}
	if host == "" {
		validation = append(validation, FieldError{Field: "host", Message: "is required"})
	}
	if strings.Contains(host, ",") {
		validation = append(validation, FieldError{Field: "host", Message: "multiple hosts are not supported"})
	}
	if strings.IndexFunc(host, unicode.IsSpace) >= 0 {
		validation = append(validation, FieldError{Field: "host", Message: "must not contain whitespace"})
	}
	if containsControl(host) {
		validation = append(validation, FieldError{Field: "host", Message: "must not contain control characters"})
	}
	if utf8.RuneCountInString(host) > 1024 {
		validation = append(validation, FieldError{Field: "host", Message: "must be 1024 characters or fewer"})
	}
	if database == "" {
		validation = append(validation, FieldError{Field: "database", Message: "is required"})
	}
	if containsControl(database) {
		validation = append(validation, FieldError{Field: "database", Message: "must not contain control characters"})
	}
	if utf8.RuneCountInString(database) > 256 {
		validation = append(validation, FieldError{Field: "database", Message: "must be 256 characters or fewer"})
	}
	if user == "" {
		validation = append(validation, FieldError{Field: "user", Message: "is required"})
	}
	if containsControl(user) {
		validation = append(validation, FieldError{Field: "user", Message: "must not contain control characters"})
	}
	if utf8.RuneCountInString(user) > 256 {
		validation = append(validation, FieldError{Field: "user", Message: "must be 256 characters or fewer"})
	}
	if draft.ReplacePassword && len(draft.Password) > 4096 {
		validation = append(validation, FieldError{Field: "password", Message: "must be 4096 bytes or fewer"})
	}
	port, err := strconv.ParseUint(strings.TrimSpace(draft.Port), 10, 16)
	if err != nil || port == 0 {
		validation = append(validation, FieldError{Field: "port", Message: "must be between 1 and 65535"})
	}
	if !validSSLModes[sslMode] {
		validation = append(validation, FieldError{Field: "ssl_mode", Message: "must be disable, allow, prefer, require, verify-ca, or verify-full"})
	}

	for _, saved := range existing.Profiles {
		if saved.ID != draft.ID && strings.EqualFold(strings.TrimSpace(saved.Name), name) {
			validation = append(validation, FieldError{Field: "name", Message: "must be unique"})
			break
		}
	}

	parameters := make(map[string]string, len(draft.AdvancedParameters))
	if len(draft.AdvancedParameters) > 64 {
		validation = append(validation, FieldError{Field: "advanced", Message: "must contain 64 parameters or fewer"})
	}
	for index, parameter := range draft.AdvancedParameters {
		parameterName := strings.ToLower(strings.TrimSpace(parameter.Name))
		field := fmt.Sprintf("advanced[%d]", index)
		if parameterName == "" {
			validation = append(validation, FieldError{Field: field, Message: "parameter name is required"})
			continue
		}
		if reservedParameters[parameterName] {
			validation = append(validation, FieldError{Field: field, Message: "cannot redefine a core connection field"})
			continue
		}
		lowerValue := strings.ToLower(parameter.Value)
		if strings.Contains(parameterName, "password") || strings.Contains(parameterName, "passfile") ||
			strings.Contains(parameterName, "secret") || strings.Contains(lowerValue, "password=") ||
			strings.Contains(lowerValue, "passfile=") {
			validation = append(validation, FieldError{Field: field, Message: "must not contain a password or secret"})
			continue
		}
		if !validParameterName(parameterName) {
			validation = append(validation, FieldError{Field: field, Message: "parameter name may contain only letters, numbers, dots, underscores, and hyphens"})
			continue
		}
		if utf8.RuneCountInString(parameterName) > 128 {
			validation = append(validation, FieldError{Field: field, Message: "parameter name must be 128 characters or fewer"})
			continue
		}
		if containsControl(parameter.Value) {
			validation = append(validation, FieldError{Field: field, Message: "parameter value must not contain control characters"})
			continue
		}
		if len(parameter.Value) > 8192 {
			validation = append(validation, FieldError{Field: field, Message: "parameter value must be 8192 bytes or fewer"})
			continue
		}
		if _, exists := parameters[parameterName]; exists {
			validation = append(validation, FieldError{Field: field, Message: "parameter names must be unique"})
			continue
		}
		parameters[parameterName] = parameter.Value
	}

	if len(validation) != 0 {
		return Profile{}, validation
	}
	return Profile{
		ID:                 draft.ID,
		Name:               name,
		Host:               host,
		Port:               uint16(port),
		Database:           database,
		User:               user,
		SSLMode:            sslMode,
		AdvancedParameters: parameters,
		SavePassword:       draft.SavePassword,
	}, nil
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func validParameterName(name string) bool {
	for _, character := range name {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("._-", character) {
			continue
		}
		return false
	}
	return true
}

func sortedParameters(parameters map[string]string) []Parameter {
	names := make([]string, 0, len(parameters))
	for name := range parameters {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Parameter, 0, len(names))
	for _, name := range names {
		result = append(result, Parameter{Name: name, Value: parameters[name]})
	}
	return result
}

func formatPort(port uint16) string {
	return strconv.FormatUint(uint64(port), 10)
}
