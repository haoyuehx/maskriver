package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func TestScanSQLChecksExtraction(t *testing.T) {
	tests := []struct {
		name string
		ddl  string
		want []string
		ok   bool
	}{
		{
			name: "column level",
			ddl:  `CREATE TABLE t (id INTEGER PRIMARY KEY, email TEXT NOT NULL CHECK (length(email) >= 6))`,
			want: []string{"( length ( email ) >= 6 )"},
			ok:   true,
		},
		{
			name: "table level and named constraint keep declaration order",
			ddl:  `CREATE TABLE t (a INTEGER, CHECK (a > 0 AND a < 10), CONSTRAINT c2 CHECK (a != 5))`,
			want: []string{"( a > 0 AND a < 10 )", "( a != 5 )"},
			ok:   true,
		},
		{
			name: "string literal containing CHECK and parens is one token",
			ddl:  `CREATE TABLE t (x TEXT CHECK (x <> 'CHECK (fake)'))`,
			want: []string{"( x <> 'CHECK (fake)' )"},
			ok:   true,
		},
		{
			name: "doubled quote escape inside literal",
			ddl:  `CREATE TABLE t (x TEXT CHECK (x <> 'it''s CHECK (x)'))`,
			want: []string{"( x <> 'it''s CHECK (x)' )"},
			ok:   true,
		},
		{
			name: "commented out CHECK is ignored",
			ddl:  `CREATE TABLE t (id INTEGER, /* CHECK (id > 0) */ x TEXT) -- CHECK (x IS NOT NULL)`,
			want: nil,
			ok:   true,
		},
		{
			name: "quoted identifier named check is not a constraint",
			ddl:  "CREATE TABLE t ([check] TEXT, \"check\" INT)",
			want: nil,
			ok:   true,
		},
		{
			name: "nested parentheses in expression",
			ddl:  `CREATE TABLE t (x INT CHECK ((x + 1) * (x - 2) > 0))`,
			want: []string{"( ( x + 1 ) * ( x - 2 ) > 0 )"},
			ok:   true,
		},
		{
			name: "unterminated literal is ambiguous",
			ddl:  `CREATE TABLE t (x TEXT CHECK (x = 'unterminated)`,
			ok:   false,
		},
		{
			name: "unterminated block comment is ambiguous",
			ddl:  `CREATE TABLE t (x TEXT) /* unterminated`,
			ok:   false,
		},
		{
			name: "bare CHECK without parens is ambiguous",
			ddl:  `CREATE TABLE t (check TEXT)`,
			ok:   false,
		},
		{
			name: "unbalanced parentheses are ambiguous",
			ddl:  `CREATE TABLE t (x INT CHECK (x > 0)`,
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := scanSQLChecks(tt.ddl)
			if ok != tt.ok {
				t.Fatalf("ok=%t, want %t (checks=%v)", ok, tt.ok, got)
			}
			if !ok {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("checks=%v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("checks[%d]=%q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func sqliteChecksOptions(t *testing.T, ddl ...string) contracts.DatabaseOptions {
	t.Helper()
	path := filepath.Join(t.TempDir(), "checks.db")
	handle, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range ddl {
		if _, err = handle.Exec(statement); err != nil {
			_ = handle.Close()
			t.Fatalf("ddl: %v", err)
		}
	}
	if err = handle.Close(); err != nil {
		t.Fatal(err)
	}
	value, err := contracts.NewValue(contracts.Text, path)
	if err != nil {
		t.Fatal(err)
	}
	return contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: value, DatasetID: "checks",
		Schemas: []string{"main"}, MaxOpenConns: 1, ConnectTimeout: 5 * time.Second,
	}
}

func TestSQLiteDescribeReportsCheckCoverage(t *testing.T) {
	ctx := context.Background()
	opts := sqliteChecksOptions(t,
		`CREATE TABLE guarded (
			id INTEGER PRIMARY KEY,
			email TEXT NOT NULL,
			CHECK (length(email) >= 6 AND email LIKE '%@%')
		)`,
		`CREATE TABLE plain (id INTEGER PRIMARY KEY, note TEXT)`,
	)
	reader, err := OpenReader(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	guarded, err := reader.Describe(ctx, contracts.TableRef{Database: "checks", Schema: "main", Table: "guarded"})
	if err != nil {
		t.Fatal(err)
	}
	if !guarded.Coverage.Checks {
		t.Fatalf("expected CHECK coverage for parseable DDL, got %+v", guarded.Coverage)
	}
	if len(guarded.Checks) != 1 {
		t.Fatalf("checks=%v, want exactly one", guarded.Checks)
	}

	plain, err := reader.Describe(ctx, contracts.TableRef{Database: "checks", Schema: "main", Table: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	if !plain.Coverage.Checks || len(plain.Checks) != 0 {
		t.Fatalf("table without CHECK must report covered/empty: coverage=%+v checks=%v", plain.Coverage, plain.Checks)
	}
}
