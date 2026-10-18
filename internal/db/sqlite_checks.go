package db

import (
	"context"
	"database/sql"
	"strings"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// sqliteChecks extracts CHECK expressions from the stored CREATE TABLE
// statement. SQLite exposes no structured CHECK metadata, so the DDL text is
// the only source. The scan is deliberately conservative: any construct it
// cannot tokenise unambiguously reports coverage=false instead of guessing.
func (e *adapter) sqliteChecks(ctx context.Context, ref contracts.TableRef) ([]string, bool, error) {
	var ddl sql.NullString
	row := e.db.QueryRowContext(ctx,
		"SELECT sql FROM main.sqlite_master WHERE type='table' AND name=?", ref.Table)
	if err := row.Scan(&ddl); err != nil {
		return nil, false, dbErrorOrCategory(err)
	}
	if !ddl.Valid {
		// Defensive only: SQLite normally stores DDL for virtual and shadow
		// tables too. If the text is missing, report the CHECK class as
		// unverified instead of failing the whole Describe.
		return nil, false, nil
	}
	checks, ok := scanSQLChecks(ddl.String)
	return checks, ok, nil
}

// scanSQLChecks returns the CHECK expressions in declaration order and whether
// the whole statement was scanned without ambiguity.
func scanSQLChecks(ddl string) ([]string, bool) {
	tokens, ok := scanSQLTokens(ddl)
	if !ok {
		return nil, false
	}
	var checks []string
	for i := 0; i < len(tokens); i++ {
		if !strings.EqualFold(tokens[i], "CHECK") {
			continue
		}
		if i+1 >= len(tokens) || tokens[i+1] != "(" {
			// A bare CHECK that is not followed by a parenthesised expression is
			// either a quoted-name edge case we do not model or malformed DDL.
			return nil, false
		}
		end, ok := matchParen(tokens, i+1)
		if !ok {
			return nil, false
		}
		checks = append(checks, strings.Join(tokens[i+1:end+1], " "))
		i = end
	}
	return checks, true
}

// scanSQLTokens splits SQL into words, whole quoted literals/identifiers and
// single-character punctuation. Comments and whitespace are dropped. It reports
// false for unterminated strings, quoted names or block comments so callers can
// fail closed rather than compare partially parsed text.
func scanSQLTokens(sql string) ([]string, bool) {
	tokens := make([]string, 0, len(sql)/4)
	depth := 0
	for i := 0; i < len(sql); {
		c := sql[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			newline := strings.IndexByte(sql[i:], '\n')
			if newline < 0 {
				// Comment to end of input is complete, but the statement balance
				// still has to hold.
				return tokens, depth == 0
			}
			i += newline + 1
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return nil, false
			}
			i += end + 4
		case c == '\'' || c == '"' || c == '`':
			end, ok := scanQuotedRun(sql, i, c)
			if !ok {
				return nil, false
			}
			tokens = append(tokens, sql[i:end])
			i = end
		case c == '[':
			end := strings.IndexByte(sql[i:], ']')
			if end < 0 {
				return nil, false
			}
			tokens = append(tokens, sql[i:i+end+1])
			i += end + 1
		case isWordStart(c):
			end := i + 1
			for end < len(sql) && isWordPart(sql[end]) {
				end++
			}
			tokens = append(tokens, sql[i:end])
			i = end
		case c >= '0' && c <= '9':
			end := scanNumber(sql, i)
			tokens = append(tokens, sql[i:end])
			i = end
		case isOperatorByte(c) && i+1 < len(sql) && isOperatorByte(sql[i+1]):
			tokens = append(tokens, sql[i:i+2])
			i += 2
		default:
			switch c {
			case '(':
				depth++
			case ')':
				depth--
				if depth < 0 {
					return nil, false
				}
			}
			tokens = append(tokens, string(c))
			i++
		}
	}
	if depth != 0 {
		return nil, false
	}
	return tokens, true
}

// scanQuotedRun returns the index just past a quoted run that starts at i. The
// quote character is escaped by doubling it, as SQLite accepts for ' " and `.
func scanQuotedRun(sql string, i int, quote byte) (int, bool) {
	for j := i + 1; j < len(sql); j++ {
		if sql[j] != quote {
			continue
		}
		if j+1 < len(sql) && sql[j+1] == quote {
			j++
			continue
		}
		return j + 1, true
	}
	return 0, false
}

// matchParen returns the index of the ")" matching the "(" at open.
func matchParen(tokens []string, open int) (int, bool) {
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch tokens[i] {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i, true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

func isWordStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isOperatorByte lists characters that form two-character SQL operators, so
// ">=" is one token instead of two. This keeps normalised expressions readable
// and avoids accidental collisions between adjacent punctuation.
func isOperatorByte(c byte) bool {
	switch c {
	case '>', '<', '!', '=', '|', ':':
		return true
	}
	return false
}

// scanNumber returns the end of a numeric literal starting at i.
func scanNumber(sql string, i int) int {
	j := i
	for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
		j++
	}
	if j < len(sql) && sql[j] == '.' {
		j++
		for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
			j++
		}
	}
	return j
}

func isWordPart(c byte) bool {
	return isWordStart(c) || (c >= '0' && c <= '9') || c == '$'
}
