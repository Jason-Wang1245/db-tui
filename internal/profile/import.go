package profile

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type ImportResult struct {
	Draft    Draft
	Password string
}

func ImportURL(raw string) (ImportResult, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 64<<10 {
		return ImportResult{}, fmt.Errorf("connection URL is too large")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ImportResult{}, fmt.Errorf("connection URL is malformed")
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return ImportResult{}, fmt.Errorf("connection URL scheme must be postgres or postgresql")
	}
	if parsed.User == nil || parsed.User.Username() == "" {
		return ImportResult{}, fmt.Errorf("connection URL user is required")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return ImportResult{}, fmt.Errorf("connection URL host is required")
	}
	if strings.Contains(parsed.Host, ",") || strings.Contains(parsed.Hostname(), ",") {
		return ImportResult{}, fmt.Errorf("connection URLs with multiple hosts are not supported")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return ImportResult{}, fmt.Errorf("connection URL database is required")
	}
	if parsed.Fragment != "" {
		return ImportResult{}, fmt.Errorf("connection URL fragments are not supported")
	}

	port := "5432"
	if parsed.Port() != "" {
		value, err := strconv.ParseUint(parsed.Port(), 10, 16)
		if err != nil || value == 0 {
			return ImportResult{}, fmt.Errorf("connection URL port must be between 1 and 65535")
		}
		port = parsed.Port()
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return ImportResult{}, fmt.Errorf("connection URL parameters are malformed")
	}
	seenParameters := make(map[string]bool, len(query))
	for name, values := range query {
		if len(values) != 1 {
			return ImportResult{}, fmt.Errorf("each connection parameter must appear only once")
		}
		normalized := strings.ToLower(strings.TrimSpace(name))
		if seenParameters[normalized] {
			return ImportResult{}, fmt.Errorf("each connection parameter must appear only once")
		}
		seenParameters[normalized] = true
	}
	for name := range query {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if reservedParameters[normalized] && normalized != "sslmode" {
			return ImportResult{}, fmt.Errorf("connection parameters cannot override core URL fields")
		}
	}

	sslMode := "prefer"
	for name, values := range query {
		if strings.EqualFold(name, "sslmode") {
			sslMode = values[0]
			delete(query, name)
		}
	}
	parameters := make(map[string]string, len(query))
	for name, values := range query {
		parameters[name] = values[0]
	}
	password, passwordSet := parsed.User.Password()
	database, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil {
		return ImportResult{}, fmt.Errorf("connection URL database is malformed")
	}
	if database == "" || strings.Contains(database, "/") {
		return ImportResult{}, fmt.Errorf("connection URL must name exactly one database")
	}
	draft := NewDraft()
	draft.Name = database + "@" + parsed.Hostname()
	draft.Host = parsed.Hostname()
	draft.Port = port
	draft.Database = database
	draft.User = parsed.User.Username()
	draft.Password = password
	draft.ReplacePassword = passwordSet
	draft.SSLMode = sslMode
	draft.AdvancedParameters = sortedParameters(parameters)
	if _, err := ValidateDraft(draft, Document{Version: CurrentDocumentVersion}); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Draft: draft, Password: password}, nil
}
