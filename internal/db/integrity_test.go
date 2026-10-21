package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func TestSQLiteIntegrityCheckDetectsForeignKeyViolation(t *testing.T) {
	path := t.TempDir() + "/integrity.db"
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`PRAGMA foreign_keys=OFF`,
		`CREATE TABLE parent (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parent(id))`,
		`INSERT INTO child VALUES (1, 99)`,
	} {
		if _, err := raw.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()
	opts := contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: testValue(t, contracts.Text, path),
		DatasetID: "maskriver", Schemas: []string{"main"}, MaxOpenConns: 1,
		ConnectTimeout: 5 * time.Second,
	}
	r, err := OpenReader(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	checker, ok := r.(interface {
		IntegrityCheck(context.Context, contracts.TableRef) error
	})
	if !ok {
		t.Fatal("reader does not expose IntegrityCheck")
	}
	ref := contracts.TableRef{Database: "maskriver", Schema: "main", Table: "child"}
	if err := checker.IntegrityCheck(context.Background(), ref); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("foreign-key violation: got %v, want ErrConflict", err)
	}
}
