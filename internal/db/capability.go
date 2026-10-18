package db

import (
	"context"
	"strings"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// This file holds the internal read-only capability probes that the frozen
// m1a-v1 contract surface (pkg/contracts) deliberately does not model: SQLite
// journal mode and trigger existence. They exist so the runner can prove that
// a SQLite source is a quiescent snapshot and that a planned write can not fire
// a trigger. They are read-only, never persist anything and return sanitized
// category errors only.

// Quiescent reports whether this read-only session can prove that the main
// database file alone holds the complete committed database state.
//
// SourceSnapshotID is the SHA-256 of the main .db file. In WAL mode committed
// transactions stay in the -wal sidecar until a checkpoint, so the main file
// can keep exactly the same bytes while the logical content changes: a
// regression fixture measured an unchanged 8192-byte main file with an
// identical SHA-256 across an uncheckpointed INSERT, while a fresh read-only
// session already saw the new row. Hashing .db plus -wal and -shm is not an
// atomic snapshot either, because the three files are not read under one lock,
// so this probe refuses every mode that can keep committed data outside the
// main file instead of claiming one.
//
// A hot rollback journal never reaches this probe: a read-only session can not
// roll it back, so opening or describing the database already fails closed.
func (a *adapter) Quiescent(ctx context.Context) error {
	if err := a.check(ctx); err != nil {
		return err
	}
	if a.dialect != contracts.SQLite {
		return contracts.ErrUnsupported
	}
	var mode string
	if err := a.db.QueryRowContext(ctx, "PRAGMA main.journal_mode").Scan(&mode); err != nil {
		return dbError(err)
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "delete", "truncate", "persist", "memory", "off":
		return nil
	}
	// "wal" keeps committed frames outside the main file until a checkpoint, and
	// an unknown future mode can not be assumed to behave differently.
	return contracts.ErrIncomplete
}

// HasTriggers reports whether the SQLite table has at least one trigger.
//
// TableSchema carries no trigger metadata, so identical source and target
// schemas can not prove that a table has no triggers, and SQLite's DDL does not
// attribute a trigger to a column. A trigger body is arbitrary SQL that can
// raise, rewrite NEW/OLD or write other rows, so a caller that writes the table
// must refuse any trigger on it rather than guess which columns it reacts to.
func (a *adapter) HasTriggers(ctx context.Context, ref contracts.TableRef) (bool, error) {
	if err := a.check(ctx); err != nil {
		return false, err
	}
	if err := a.table(ref); err != nil {
		return false, err
	}
	if a.dialect != contracts.SQLite {
		return false, contracts.ErrUnsupported
	}
	var count int
	err := a.db.QueryRowContext(ctx,
		// SQLite stores the table name exactly as written in the trigger DDL, so
		// a trigger declared ON PEOPLE for table people would otherwise be
		// missed. Compare case-insensitively.
		"SELECT COUNT(*) FROM main.sqlite_schema WHERE type='trigger' AND tbl_name=? COLLATE NOCASE", ref.Table).Scan(&count)
	if err != nil {
		return false, dbError(err)
	}
	return count > 0, nil
}

// CheckMentionsColumn reports whether a stored CHECK expression may reference
// column. It reuses the conservative SQL scanner that extracted the expression,
// and an expression the scanner can not tokenise counts as a reference. A match
// inside a string literal also counts: the cost of a false positive is a
// refused plan, never a wrong write.
func CheckMentionsColumn(expression, column string) bool {
	if column == "" {
		return false
	}
	tokens, ok := scanSQLTokens(expression)
	if !ok {
		return true
	}
	for _, token := range tokens {
		if strings.EqualFold(unquoteSQLToken(token), column) {
			return true
		}
	}
	return false
}

// unquoteSQLToken strips the quoting SQLite accepts around an identifier or a
// string literal: '...', "...", `...` and [...].
func unquoteSQLToken(token string) string {
	if len(token) < 2 {
		return token
	}
	first, last := token[0], token[len(token)-1]
	switch {
	case first == '\'' && last == '\'':
		return token[1 : len(token)-1]
	case first == '"' && last == '"':
		return token[1 : len(token)-1]
	case first == '`' && last == '`':
		return token[1 : len(token)-1]
	case first == '[' && last == ']':
		return token[1 : len(token)-1]
	}
	return token
}
