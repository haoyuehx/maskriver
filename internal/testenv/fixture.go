// Package testenv is Main-owned synthetic environment preparation, not a DB adapter.
package testenv

import (
	"context"
	"database/sql"
	"fmt"
)

const Rows = 25

// Seed initializes an empty disposable database. Never call against an existing
// or production database. Deliberately no DROP/IF NOT EXISTS or arbitrary SQL API.
func Seed(ctx context.Context, db *sql.DB, dialect string) error {
	var statements []string
	switch dialect {
	case "sqlite":
		statements = []string{
			`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE, nickname TEXT NULL, amount TEXT NOT NULL, born DATE NOT NULL, payload BLOB NOT NULL, CHECK (id > 0))`,
			`CREATE INDEX people_born ON people(born)`,
			`CREATE TABLE links (tenant INTEGER NOT NULL, seq INTEGER NOT NULL, person_id INTEGER NOT NULL, PRIMARY KEY(tenant,seq), FOREIGN KEY(person_id) REFERENCES people(id))`,
			`CREATE TABLE keyless (note TEXT)`,
		}
	case "mysql":
		statements = []string{
			`CREATE TABLE people (id BIGINT PRIMARY KEY, email VARCHAR(128) NOT NULL UNIQUE, nickname VARCHAR(128) NULL, amount DECIMAL(24,4) NOT NULL, born DATE NOT NULL, payload BLOB NOT NULL, CHECK (id > 0)) ENGINE=InnoDB`,
			`CREATE INDEX people_born ON people(born)`,
			`CREATE TABLE links (tenant BIGINT NOT NULL, seq BIGINT NOT NULL, person_id BIGINT NOT NULL, PRIMARY KEY(tenant,seq), FOREIGN KEY(person_id) REFERENCES people(id)) ENGINE=InnoDB`,
			`CREATE TABLE keyless (note TEXT) ENGINE=InnoDB`,
		}
	default:
		return fmt.Errorf("unsupported fixture dialect")
	}
	// MySQL DDL auto-commits; callers discard the entire instance on failure.
	for _, s := range statements {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("fixture schema failed")
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("fixture begin failed")
	}
	defer tx.Rollback()
	for i := 1; i <= Rows; i++ {
		var nickname any = fmt.Sprintf("合成样本-%02d", i)
		if i == 1 {
			nickname = nil
		}
		if i == 2 {
			nickname = ""
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO people(id,email,nickname,amount,born,payload) VALUES(?,?,?,?,?,?)`, i, fmt.Sprintf("fixture-%02d@example.invalid", i), nickname, "12345678901234567890.1200", "2000-02-29", []byte{0, 1, 255}); err != nil {
			return fmt.Errorf("fixture people insert failed")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO links(tenant,seq,person_id) VALUES(?,?,?)`, (i-1)/10+1, (i-1)%10+1, i); err != nil {
			return fmt.Errorf("fixture links insert failed")
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO keyless(note) VALUES(?)`, "synthetic-only"); err != nil {
		return fmt.Errorf("fixture keyless insert failed")
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("fixture commit failed")
	}
	return nil
}
