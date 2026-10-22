// Package mysqlparam centralizes the DSN parameter policy shared by the CLI
// config and the database adapter. The go-sql-driver applies Params as session
// variables, so an unvalidated key or comma-containing value is a SET-clause
// injection, not merely a connection option.
package mysqlparam

import (
	"regexp"
	"strings"
)

var keyPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Unsafe reports whether one DSN parameter must be refused. Keys are normalized
// for MySQL scope aliases (`@@session.`, `@@local.`, `SESSION `, `GLOBAL `)
// before matching; values containing SET separators are refused.
func Unsafe(key, value string) bool {
	if strings.ContainsAny(value, ",;") {
		return true
	}
	normalized := normalizeKey(key)
	if !keyPattern.MatchString(normalized) {
		return true
	}
	switch normalized {
	case "transaction_read_only", "tx_read_only", "foreign_key_checks",
		"sql_mode", "init_connect", "autocommit", "read_only",
		"super_read_only", "transaction_isolation", "tx_isolation",
		"unique_checks", "check_constraint_checks", "session_track_transaction_info":
		return true
	}
	return false
}

func normalizeKey(key string) string {
	k := strings.TrimSpace(strings.ToLower(key))
	k = strings.TrimPrefix(k, "@@")
	for _, scope := range []string{"session.", "local.", "global."} {
		k = strings.TrimPrefix(k, scope)
	}
	for _, scope := range []string{"session ", "local ", "global "} {
		k = strings.TrimPrefix(k, scope)
	}
	return strings.TrimSpace(k)
}
