// Package db implements the frozen Reader and Writer contracts for SQLite and MySQL.
//
// The adapter reports unsupported or unknown database capabilities explicitly.
// It does not manage production credentials, application authorization, or schema changes.
package db
