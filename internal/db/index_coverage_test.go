package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// The frozen Index DTO has no sort-direction field, so a non-ascending index
// column cannot be compared. The adapter must report the index class as
// unverified rather than claim it matched.
func TestSQLiteDescribeMarksDescendingIndexUnverified(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		ddl  string
		want bool
	}{
		{"ascending", `CREATE TABLE t (id INTEGER PRIMARY KEY, email TEXT); CREATE INDEX t_email ON t(email ASC)`, true},
		{"implicit ascending", `CREATE TABLE t (id INTEGER PRIMARY KEY, email TEXT); CREATE INDEX t_email ON t(email)`, true},
		{"descending", `CREATE TABLE t (id INTEGER PRIMARY KEY, email TEXT); CREATE INDEX t_email ON t(email DESC)`, false},
		{"composite mixed", `CREATE TABLE t (id INTEGER PRIMARY KEY, email TEXT, name TEXT); CREATE INDEX t_mix ON t(email ASC, name DESC)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "index.db")
			handle, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = handle.Exec(tc.ddl); err != nil {
				_ = handle.Close()
				t.Fatalf("ddl: %v", err)
			}
			if err = handle.Close(); err != nil {
				t.Fatal(err)
			}
			dsn, err := contracts.NewValue(contracts.Text, path)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := OpenReader(ctx, contracts.DatabaseOptions{
				Dialect: contracts.SQLite, DSN: dsn, DatasetID: "index",
				Schemas: []string{"main"}, MaxOpenConns: 1, ConnectTimeout: 5 * time.Second,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()

			schema, err := reader.Describe(ctx, contracts.TableRef{Database: "index", Schema: "main", Table: "t"})
			if err != nil {
				t.Fatal(err)
			}
			if schema.Coverage.Indexes != tc.want {
				t.Fatalf("Coverage.Indexes=%t, want %t (indexes=%+v)", schema.Coverage.Indexes, tc.want, schema.Indexes)
			}
		})
	}
}
